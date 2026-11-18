package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"dnd-gateway/internal/diceclient"
	"dnd-gateway/internal/dicepb"
	"dnd-gateway/internal/diceserver"
	"dnd-gateway/internal/diceupstream"
	"dnd-gateway/internal/dnd"
)

func TestGatewayRoutes(t *testing.T) {
	diceUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/dice/roll" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"notation":"1d20","dice":[15],"modifier":0,"total":15}],"meta":{"count":1,"seed":7}}`))
	}))
	t.Cleanup(diceUpstream.Close)

	rulesUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/2014":
			_, _ = w.Write([]byte(`{"monsters":"/api/2014/monsters"}`))
		case "/api/2014/monsters":
			if r.URL.Query().Get("name") != "Goblin" {
				t.Errorf("query = %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`{"count":1,"results":[{"index":"goblin","name":"Goblin"}]}`))
		case "/api/2014/monsters/goblin":
			_, _ = w.Write([]byte(`{"index":"goblin","name":"Goblin","hit_points":7}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(rulesUpstream.Close)

	handler := testHandler(t, diceUpstream.URL, rulesUpstream.URL)

	t.Run("health", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok"`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dnd detail", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dnd/monsters/goblin", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"hit_points":7`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dnd list keeps query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dnd/monsters?name=Goblin", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"count":1`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("rejects unsafe slug", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dnd/bad_name", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("dice roll through grpc", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/v1/dice/roll?notation=d20&seed=7", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"total":15`) {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("action joins both upstreams", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(`{"resource":"monsters","index":"goblin","notation":"1d20"}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%d %s", rec.Code, rec.Body.String())
		}
		var body struct {
			Resource struct {
				Name string `json:"name"`
			} `json:"resource"`
			Rolls struct {
				Data []struct {
					Total int `json:"total"`
				} `json:"data"`
			} `json:"rolls"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Resource.Name != "Goblin" || len(body.Rolls.Data) != 1 || body.Rolls.Data[0].Total != 15 {
			t.Fatalf("body %s", rec.Body.String())
		}
	})
}

func TestActionCallsUpstreamsTogether(t *testing.T) {
	started := make(chan string, 2)
	release := make(chan struct{})
	var once sync.Once
	open := func() { once.Do(func() { close(release) }) }
	t.Cleanup(open)

	wait := func(name string, ctx context.Context) {
		started <- name
		select {
		case <-release:
		case <-ctx.Done():
		}
	}

	rulesUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wait("dnd", r.Context())
		_, _ = w.Write([]byte(`{"index":"goblin"}`))
	}))
	t.Cleanup(rulesUpstream.Close)
	diceUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wait("dice", r.Context())
		_, _ = w.Write([]byte(`{"data":[{"index":0,"notation":"1d20","dice":[8],"modifier":0,"total":8}],"meta":{"seed":1,"count":1}}`))
	}))
	t.Cleanup(diceUpstream.Close)

	handler := testHandler(t, diceUpstream.URL, rulesUpstream.URL)
	req := httptest.NewRequest(http.MethodPost, "/v1/actions", strings.NewReader(`{"resource":"monsters","index":"goblin","notation":"d20"}`))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		done <- rec
	}()

	got := map[string]bool{}
	timeout := time.After(2 * time.Second)
	for len(got) < 2 {
		select {
		case name := <-started:
			got[name] = true
		case <-timeout:
			t.Fatalf("upstream calls did not overlap: %v", got)
		}
	}
	open()

	rec := <-done
	if rec.Code != http.StatusOK {
		body, _ := io.ReadAll(rec.Body)
		t.Fatalf("%d %s", rec.Code, body)
	}
}

func testHandler(t *testing.T, diceBase, dndBase string) http.Handler {
	t.Helper()
	upstream, err := diceupstream.New(diceBase)
	if err != nil {
		t.Fatal(err)
	}
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := grpc.NewServer(dicepb.ServerOptions()...)
	dicepb.RegisterDiceServiceServer(srv, diceserver.New(upstream))
	go srv.Serve(lis)
	t.Cleanup(srv.Stop)
	t.Cleanup(func() { _ = lis.Close() })

	dialOpts := append(dicepb.DialOptions(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	conn, err := grpc.NewClient("dns:///"+lis.Addr().String(), dialOpts...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	rules, err := dnd.New(dndBase, "2014")
	if err != nil {
		t.Fatal(err)
	}
	return New(rules, diceclient.New(dicepb.NewDiceServiceClient(conn)), log.New(io.Discard, "", 0))
}
