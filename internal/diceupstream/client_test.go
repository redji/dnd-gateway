package diceupstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"dnd-gateway/internal/dice"
)

func TestRollEncodesPlus(t *testing.T) {
	var got string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"index":0,"notation":"2d6+3","dice":[3,5],"modifier":3,"total":11}],"meta":{"count":1,"seed":42}}`))
	}))
	t.Cleanup(upstream.Close)

	client, err := New(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	seed := int64(42)
	result, err := client.Roll(context.Background(), dice.Request{Notation: "2d6 3", Count: 0, Seed: &seed})
	if err != nil {
		t.Fatal(err)
	}
	if result.Notation != "2d6+3" || len(result.Rolls) != 1 || result.Rolls[0].Total != 11 || result.Seed != 42 {
		t.Fatalf("result %+v", result)
	}
	if want := "count=1&notation=2d6%2B3&seed=42"; got != want {
		t.Fatalf("query %s", got)
	}
}

func TestRollForwardsUpstreamStatus(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusBadRequest)
	}))
	t.Cleanup(upstream.Close)

	client, err := New(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Roll(context.Background(), dice.Request{Notation: "d20"})
	var status *StatusError
	if !errorAs(err, &status) || status.Status != http.StatusBadRequest {
		t.Fatalf("err = %v", err)
	}
}

func errorAs(err error, target **StatusError) bool {
	if err == nil {
		return false
	}
	se, ok := err.(*StatusError)
	if !ok {
		return false
	}
	*target = se
	return true
}
