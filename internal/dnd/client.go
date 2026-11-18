package dnd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const maxBody = 4 << 20

var (
	// ErrBadSegment means a resource or index is not a single URL slug.
	ErrBadSegment = errors.New("invalid resource name")
	// ErrLongQuery means the forwarded query string is over the gateway limit.
	ErrLongQuery = errors.New("query is too long")
)

var segmentRE = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Client reads the public D&D 5e API. Paths are built from a fixed base URL
// and checked resource names, so callers cannot point the client at another host.
type Client struct {
	base    *url.URL
	version string
	http    *http.Client
}

func New(baseURL, version string) (*Client, error) {
	if version != "2014" && version != "2024" {
		return nil, fmt.Errorf("dnd api version must be 2014 or 2024")
	}
	u, err := url.Parse(strings.TrimRight(baseURL, "/"))
	if err != nil || u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("dnd base URL must be absolute")
	}
	host := u.Host
	return &Client{
		base:    u,
		version: version,
		http: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				if len(via) >= 3 {
					return fmt.Errorf("too many redirects")
				}
				if req.URL.Host != host {
					return fmt.Errorf("redirect left the dnd upstream")
				}
				return nil
			},
		},
	}, nil
}

// Response is the upstream status and body, forwarded by the gateway.
type Response struct {
	Status int
	Body   []byte
}

func (c *Client) Index(ctx context.Context) (Response, error) {
	return c.get(ctx, c.base.JoinPath("api", c.version), "")
}

func (c *Client) List(ctx context.Context, resource, rawQuery string) (Response, error) {
	resource = strings.ToLower(resource)
	if err := checkSegment(resource); err != nil {
		return Response{}, err
	}
	if len(rawQuery) > 2048 {
		return Response{}, ErrLongQuery
	}
	return c.get(ctx, c.base.JoinPath("api", c.version, resource), rawQuery)
}

func (c *Client) Get(ctx context.Context, resource, index string) (Response, error) {
	resource = strings.ToLower(resource)
	index = strings.ToLower(index)
	if err := checkSegment(resource); err != nil {
		return Response{}, err
	}
	if err := checkSegment(index); err != nil {
		return Response{}, err
	}
	return c.get(ctx, c.base.JoinPath("api", c.version, resource, index), "")
}

func (c *Client) get(ctx context.Context, u *url.URL, rawQuery string) (Response, error) {
	if rawQuery != "" {
		u.RawQuery = rawQuery
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Response{}, err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "dnd-gateway/0.1")

	resp, err := c.http.Do(req)
	if err != nil {
		return Response{}, fmt.Errorf("dnd upstream: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		return Response{}, fmt.Errorf("dnd upstream: %w", err)
	}
	if len(body) > maxBody {
		return Response{}, fmt.Errorf("dnd upstream response is too large")
	}
	return Response{Status: resp.StatusCode, Body: body}, nil
}

func checkSegment(s string) error {
	if !segmentRE.MatchString(s) || len(s) > 128 {
		return ErrBadSegment
	}
	return nil
}
