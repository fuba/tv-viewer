package recorder

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestClientReserveProgram(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/record" {
			t.Fatalf("path = %q, want /api/record", r.URL.Path)
		}
		if got := r.URL.Query().Get("program_id"); got != "323912360812345" {
			t.Fatalf("program_id = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"success","message":"URL registered for recording"}`))
	}))
	defer server.Close()

	if err := NewClient(server.URL).ReserveProgram(context.Background(), 323912360812345); err != nil {
		t.Fatal(err)
	}
}

func TestClientDeduplicatesSuccessfulReservation(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write([]byte(`{"status":"success"}`))
	}))
	defer server.Close()

	client := NewClient(server.URL)
	for range 2 {
		if err := client.ReserveProgram(context.Background(), 123); err != nil {
			t.Fatal(err)
		}
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("upstream requests = %d, want 1", got)
	}
}

func TestClientReserveProgramRejectsUpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "write failed", http.StatusInternalServerError)
	}))
	defer server.Close()

	err := NewClient(server.URL).ReserveProgram(context.Background(), 1)
	if err == nil {
		t.Fatal("expected upstream error")
	}
}
