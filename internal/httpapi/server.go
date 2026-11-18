package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"

	"dnd-gateway/internal/dice"
	"dnd-gateway/internal/diceclient"
	"dnd-gateway/internal/dnd"
)

// Handler is the public REST gateway.
type Handler struct {
	dnd   *dnd.Client
	rolls dice.Roller
	log   *log.Logger
}

func New(dndClient *dnd.Client, rolls dice.Roller, logger *log.Logger) http.Handler {
	if logger == nil {
		logger = log.Default()
	}
	h := &Handler{dnd: dndClient, rolls: rolls, log: logger}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /v1", h.catalog)
	mux.HandleFunc("GET /v1/dnd", h.dndIndex)
	mux.HandleFunc("GET /v1/dnd/{resource}", h.dndList)
	mux.HandleFunc("GET /v1/dnd/{resource}/{index}", h.dndGet)
	mux.HandleFunc("GET /v1/dice/roll", h.roll)
	mux.HandleFunc("POST /v1/actions", h.action)
	return logRequests(logger, mux)
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *Handler) catalog(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"service": "dnd-gateway",
		"endpoints": []string{
			"GET /health",
			"GET /v1/dnd",
			"GET /v1/dnd/{resource}",
			"GET /v1/dnd/{resource}/{index}",
			"GET /v1/dice/roll?notation=2d6+3&count=1",
			"POST /v1/actions",
		},
	})
}

func (h *Handler) dndIndex(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dnd.Index(r.Context())
	if err != nil {
		h.log.Printf("dnd index: %v", err)
		writeError(w, http.StatusBadGateway, "dnd upstream is unavailable")
		return
	}
	writeUpstream(w, resp)
}

func (h *Handler) dndList(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dnd.List(r.Context(), r.PathValue("resource"), r.URL.RawQuery)
	if err != nil {
		writeClientOrGateway(w, err)
		return
	}
	writeUpstream(w, resp)
}

func (h *Handler) dndGet(w http.ResponseWriter, r *http.Request) {
	resp, err := h.dnd.Get(r.Context(), r.PathValue("resource"), r.PathValue("index"))
	if err != nil {
		writeClientOrGateway(w, err)
		return
	}
	writeUpstream(w, resp)
}

func (h *Handler) roll(w http.ResponseWriter, r *http.Request) {
	req, err := rollRequestFromQuery(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.rolls.Roll(r.Context(), req)
	if err != nil {
		writeRollError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rollBody(result))
}

type actionRequest struct {
	Resource string `json:"resource"`
	Index    string `json:"index"`
	Notation string `json:"notation"`
	Count    int    `json:"count"`
	Seed     *int64 `json:"seed"`
}

func (h *Handler) action(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not read request body")
		return
	}
	var in actionRequest
	if err := json.Unmarshal(body, &in); err != nil {
		writeError(w, http.StatusBadRequest, "request body must be JSON")
		return
	}
	rollReq := dice.Request{Notation: in.Notation, Count: in.Count, Seed: in.Seed}
	if _, err := dice.NormalizeNotation(rollReq.Notation); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := dice.NormalizeCount(rollReq.Count); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	// Fetch the rules entry and the dice roll together. Each upstream call
	// blocks, so two goroutines keep the gateway from adding their latencies.
	var (
		wg       sync.WaitGroup
		resource dnd.Response
		resErr   error
		result   dice.Result
		rollErr  error
	)
	wg.Add(2)
	go func() {
		defer wg.Done()
		resource, resErr = h.dnd.Get(r.Context(), in.Resource, in.Index)
	}()
	go func() {
		defer wg.Done()
		result, rollErr = h.rolls.Roll(r.Context(), rollReq)
	}()
	wg.Wait()

	if resErr != nil {
		writeClientOrGateway(w, resErr)
		return
	}
	if resource.Status != http.StatusOK {
		writeUpstream(w, resource)
		return
	}
	if rollErr != nil {
		writeRollError(w, rollErr)
		return
	}
	if !json.Valid(resource.Body) {
		writeError(w, http.StatusBadGateway, "dnd upstream returned invalid JSON")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"resource": json.RawMessage(resource.Body),
		"rolls":    rollBody(result),
	})
}

func rollRequestFromQuery(r *http.Request) (dice.Request, error) {
	q := r.URL.Query()
	notation, err := dice.NormalizeNotation(q.Get("notation"))
	if err != nil {
		return dice.Request{}, err
	}
	count := 1
	if raw := q.Get("count"); raw != "" {
		n, convErr := strconv.Atoi(raw)
		if convErr != nil {
			return dice.Request{}, &dice.ValidationError{Msg: "count must be an integer"}
		}
		count, err = dice.NormalizeCount(n)
		if err != nil {
			return dice.Request{}, err
		}
	}
	var seed *int64
	if raw := q.Get("seed"); raw != "" {
		n, convErr := strconv.ParseInt(raw, 10, 64)
		if convErr != nil {
			return dice.Request{}, &dice.ValidationError{Msg: "seed must be an integer"}
		}
		seed = &n
	}
	return dice.Request{Notation: notation, Count: count, Seed: seed}, nil
}

func rollBody(result dice.Result) map[string]any {
	rolls := result.Rolls
	if rolls == nil {
		rolls = []dice.Roll{}
	}
	return map[string]any{
		"data": rolls,
		"meta": map[string]any{
			"notation": result.Notation,
			"count":    result.Count,
			"seed":     result.Seed,
		},
	}
}

func writeUpstream(w http.ResponseWriter, resp dnd.Response) {
	if resp.Status == 0 {
		resp.Status = http.StatusBadGateway
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.Status)
	_, _ = w.Write(resp.Body)
}

func writeClientOrGateway(w http.ResponseWriter, err error) {
	var ve *dice.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, ve.Error())
		return
	}
	if errors.Is(err, dnd.ErrBadSegment) || errors.Is(err, dnd.ErrLongQuery) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusBadGateway, "dnd upstream is unavailable")
}

func writeRollError(w http.ResponseWriter, err error) {
	var ce *diceclient.CallError
	if errors.As(err, &ce) {
		writeError(w, ce.Status, ce.Msg)
		return
	}
	var ve *dice.ValidationError
	if errors.As(err, &ve) {
		writeError(w, http.StatusBadRequest, ve.Error())
		return
	}
	writeError(w, http.StatusBadGateway, "dice service is unavailable")
}

func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func logRequests(logger *log.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		start := time.Now()
		next.ServeHTTP(sw, r)
		logger.Printf("%s %s %d %s", r.Method, r.URL.RequestURI(), sw.status, time.Since(start).Round(time.Millisecond))
	})
}
