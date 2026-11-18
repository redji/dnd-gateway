package diceupstream

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"dnd-gateway/internal/dice"
)

const maxBody = 1 << 20

// Client calls the public dice API at randomapi.dev.
type Client struct {
	base *url.URL
	http *http.Client
}

func New(baseURL string) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("dice base URL must be absolute")
	}
	host := u.Host
	return &Client{
		base: u,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				if req.URL.Host != host {
					return fmt.Errorf("redirect left the dice upstream")
				}
				return nil
			},
		},
	}, nil
}

type envelope struct {
	Data []dice.Roll `json:"data"`
	Meta struct {
		Count int   `json:"count"`
		Seed  int64 `json:"seed"`
	} `json:"meta"`
	Error string `json:"error"`
}

func (c *Client) Roll(ctx context.Context, req dice.Request) (dice.Result, error) {
	notation, err := dice.NormalizeNotation(req.Notation)
	if err != nil {
		return dice.Result{}, err
	}
	count, err := dice.NormalizeCount(req.Count)
	if err != nil {
		return dice.Result{}, err
	}

	u := c.base.JoinPath("api", "dice", "roll")
	q := u.Query()
	// Values.Encode percent-encodes "+", so a modifier is not turned into a space.
	q.Set("notation", notation)
	q.Set("count", strconv.Itoa(count))
	if req.Seed != nil {
		q.Set("seed", strconv.FormatInt(*req.Seed, 10))
	}
	u.RawQuery = q.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return dice.Result{}, err
	}
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", "dnd-gateway/0.1")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return dice.Result{}, fmt.Errorf("dice upstream: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return dice.Result{}, fmt.Errorf("dice upstream: %w", err)
	}
	if len(body) > maxBody {
		return dice.Result{}, fmt.Errorf("dice upstream response is too large")
	}
	if resp.StatusCode != http.StatusOK {
		return dice.Result{}, &StatusError{Status: resp.StatusCode, Body: clip(body)}
	}

	var env envelope
	if err := json.Unmarshal(body, &env); err != nil {
		return dice.Result{}, fmt.Errorf("dice upstream returned invalid JSON")
	}
	if env.Data == nil {
		env.Data = []dice.Roll{}
	}
	return dice.Result{
		Rolls:    env.Data,
		Seed:     env.Meta.Seed,
		Count:    count,
		Notation: notation,
	}, nil
}

// StatusError is a non-200 response from the public dice API.
type StatusError struct {
	Status int
	Body   string
}

func (e *StatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("dice upstream status %d", e.Status)
	}
	return fmt.Sprintf("dice upstream status %d: %s", e.Status, e.Body)
}

func clip(b []byte) string {
	s := strings.TrimSpace(string(b))
	if len(s) > 240 {
		return s[:240]
	}
	return s
}
