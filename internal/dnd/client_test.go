package dnd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestGetBuildsFixedPath(t *testing.T) {
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"index":"goblin","name":"Goblin"}`))
	}))
	t.Cleanup(upstream.Close)

	client, err := New(upstream.URL, "2014")
	if err != nil {
		t.Fatal(err)
	}
	resp, err := client.Get(context.Background(), "Monsters", "Goblin")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Status != http.StatusOK || !strings.Contains(string(resp.Body), "Goblin") {
		t.Fatalf("status %d body %s", resp.Status, resp.Body)
	}
	if got != "/api/2014/monsters/goblin" {
		t.Fatalf("upstream path %s", got)
	}
}

func TestRejectsUnsafeNames(t *testing.T) {
	client, err := New("https://www.dnd5eapi.co", "2014")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"../secret", "monsters/goblin", "a b", ""} {
		if _, err := client.Get(context.Background(), name, "goblin"); err == nil {
			t.Fatalf("accepted resource %q", name)
		}
	}
	long := strings.Repeat("a", 3000)
	if _, err := client.List(context.Background(), "spells", "name="+long); err != ErrLongQuery {
		t.Fatalf("query error = %v", err)
	}
}
