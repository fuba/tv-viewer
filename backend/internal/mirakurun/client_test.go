package mirakurun

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestGetServiceStreamContextCancelsPendingResponse(t *testing.T) {
	requestStarted := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(requestStarted)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := NewClient(server.URL).GetServiceStreamContext(ctx, 1024)
		done <- err
	}()
	<-requestStarted
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context cancellation, got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending stream request was not canceled")
	}
}

func TestNewClient(t *testing.T) {
	client := NewClient("http://tuner:40772")
	if client == nil {
		t.Fatal("Failed to create client")
	}
	if client.baseURL != "http://tuner:40772" {
		t.Errorf("Expected baseURL to be http://tuner:40772, got %s", client.baseURL)
	}
}

func TestGetChannels(t *testing.T) {
	client := NewClient("http://tuner:40772")

	channels, err := client.GetChannels()
	if err != nil {
		t.Logf("Failed to get channels: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}

	if len(channels) > 0 {
		t.Logf("Found %d channels", len(channels))
		for i, ch := range channels {
			if i < 3 { // Log first 3 channels
				t.Logf("Channel %d: %s (ID: %s, Type: %s)", i+1, ch.Name, ch.ID, ch.Type)
			}
		}
	}
}

func TestGetPrograms(t *testing.T) {
	client := NewClient("http://tuner:40772")

	// Test with a dummy service ID
	programs, err := client.GetPrograms(1024)
	if err != nil {
		t.Logf("Failed to get programs: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}

	t.Logf("Found %d programs", len(programs))
}

func TestGetServiceStream(t *testing.T) {
	client := NewClient("http://tuner:40772")

	// Test with a dummy service ID
	stream, err := client.GetServiceStream(1024)
	if err != nil {
		t.Logf("Failed to get service stream: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}
	defer stream.Close()

	t.Log("Successfully connected to service stream")
}

func TestGetServiceStreamKeepsLiveBodyOpen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/MP2T")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("test server does not support flushing")
			return
		}
		packet := make([]byte, 188)
		packet[0] = 0x47
		packet[3] = 0x10
		if _, err := w.Write(packet); err != nil {
			return
		}
		flusher.Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer server.Close()

	client := NewClient(server.URL)
	stream, err := client.GetServiceStream(1024)
	if err != nil {
		t.Fatalf("GetServiceStream failed: %v", err)
	}
	defer stream.Close()
	data, err := io.ReadAll(io.LimitReader(stream, 188))
	if err != nil {
		t.Fatalf("reading live body failed: %v", err)
	}
	if len(data) != 188 || data[0] != 0x47 {
		t.Fatalf("unexpected stream data: %d bytes", len(data))
	}
}

func TestChannelStreamIdentifiesTVViewer(t *testing.T) {
	gotAgent := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAgent <- r.UserAgent()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	stream, err := NewClient(server.URL).GetChannelStreamWithType("GR", "16")
	if err != nil {
		t.Fatal(err)
	}
	stream.Close()
	if got := <-gotAgent; got != "tv-viewer/1.0" {
		t.Fatalf("User-Agent = %q, want tv-viewer/1.0", got)
	}
}
