package voicetranslate

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestClientAuthenticatesStreamsPCMAndReceivesEvents(t *testing.T) {
	t.Parallel()

	receivedStart := make(chan startMessage, 1)
	receivedAudio := make(chan []byte, 1)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool {
		return r.Header.Get("Origin") == "https://honyaku.example.test"
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		_, rawStart, err := conn.ReadMessage()
		if err != nil {
			return
		}
		var start startMessage
		if err := json.Unmarshal(rawStart, &start); err != nil {
			return
		}
		receivedStart <- start
		_ = conn.WriteJSON(Event{Type: EventReady, SampleRate: 16000, Channels: 1, Format: "pcm_s16le"})

		messageType, audio, err := conn.ReadMessage()
		if err != nil || messageType != websocket.BinaryMessage {
			return
		}
		receivedAudio <- audio
		_ = conn.WriteJSON(Event{
			Type: EventFinal, CaptionID: "caption-1", Text: "hello",
			Translation: stringPtr("こんにちは"), TargetLanguage: "ja",
		})
	}))
	defer server.Close()

	config := Config{
		URL:            "ws" + strings.TrimPrefix(server.URL, "http") + "/ws",
		Origin:         "https://honyaku.example.test",
		AccessToken:    strings.Repeat("a", 32),
		SourceLanguage: "auto",
		TargetLanguage: "ja",
		SpeechEnabled:  true,
	}
	audio := make(chan []byte, 1)
	events := make(chan Event, 4)
	audio <- []byte{1, 0, 2, 0}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	errCh := make(chan error, 1)
	go func() { errCh <- NewClient(config).Run(ctx, audio, events) }()

	select {
	case start := <-receivedStart:
		if start.Type != "start" || start.AccessToken != config.AccessToken || start.TargetLanguage != "ja" || !start.SpeechEnabled {
			t.Fatalf("unexpected start message: %+v", start)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for start message")
	}

	select {
	case frame := <-receivedAudio:
		if string(frame) != string([]byte{1, 0, 2, 0}) {
			t.Fatalf("audio frame = %v", frame)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for audio frame")
	}

	for {
		select {
		case event := <-events:
			if event.Type == EventFinal {
				if event.CaptionID != "caption-1" || event.Translation == nil || *event.Translation != "こんにちは" {
					t.Fatalf("unexpected final event: %+v", event)
				}
				return
			}
		case err := <-errCh:
			if err != nil {
				t.Fatalf("client stopped before final event: %v", err)
			}
		case <-ctx.Done():
			t.Fatal("timed out waiting for final event")
		}
	}
}

func TestLiveVoiceTranslateConnection(t *testing.T) {
	if os.Getenv("VOICETRANSLATE_INTEGRATION") != "1" {
		t.Skip("set VOICETRANSLATE_INTEGRATION=1 to test the configured service")
	}
	config, err := ConfigFromEnv()
	if err != nil {
		t.Fatalf("ConfigFromEnv: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	events := make(chan Event, 2)
	result := make(chan error, 1)
	go func() { result <- NewClient(config).Run(ctx, make(chan []byte), events) }()

	select {
	case event := <-events:
		if event.Type != EventReady {
			t.Fatalf("first live event = %q, want %q", event.Type, EventReady)
		}
		cancel()
	case err := <-result:
		t.Fatalf("live VoiceTranslate connection failed before ready: %v", err)
	case <-ctx.Done():
		t.Fatalf("live VoiceTranslate ready timeout: %v", ctx.Err())
	}
	select {
	case <-result:
	case <-time.After(2 * time.Second):
		t.Fatal("live VoiceTranslate client did not stop after cancellation")
	}
}

func TestConfigRejectsInsecureRemoteWebSocket(t *testing.T) {
	t.Parallel()
	config := Config{
		URL: "ws://honyaku.example.test/ws", Origin: "https://honyaku.example.test",
		AccessToken: strings.Repeat("a", 32), TargetLanguage: "ja",
	}
	if err := config.Validate(); err == nil {
		t.Fatal("expected an insecure remote WebSocket URL to be rejected")
	}
}

func TestConfigFromEnvRequiresExplicitEndpointAndOrigin(t *testing.T) {
	t.Setenv("VOICETRANSLATE_URL", "")
	t.Setenv("VOICETRANSLATE_ORIGIN", "")
	t.Setenv("VOICETRANSLATE_ACCESS_TOKEN", strings.Repeat("a", 32))
	t.Setenv("VOICETRANSLATE_TOKEN_FILE", "")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "VOICETRANSLATE_URL") {
		t.Fatalf("ConfigFromEnv() error = %v, want required URL error", err)
	}

	t.Setenv("VOICETRANSLATE_URL", "wss://translate.example.test/ws")
	if _, err := ConfigFromEnv(); err == nil || !strings.Contains(err.Error(), "VOICETRANSLATE_ORIGIN") {
		t.Fatalf("ConfigFromEnv() error = %v, want required Origin error", err)
	}
}

func TestClientRejectsBinaryReadyEvent(t *testing.T) {
	t.Parallel()

	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}
		payload, _ := json.Marshal(Event{Type: EventReady, SampleRate: 16000, Channels: 1, Format: "pcm_s16le"})
		_ = conn.WriteMessage(websocket.BinaryMessage, payload)
	}))
	defer server.Close()

	config := Config{
		URL: "ws" + strings.TrimPrefix(server.URL, "http") + "/ws", Origin: "https://honyaku.example.test",
		AccessToken: strings.Repeat("a", 32), SourceLanguage: "auto", TargetLanguage: "ja", SpeechEnabled: true,
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := NewClient(config).Run(ctx, make(chan []byte), make(chan Event, 1))
	if err == nil || !strings.Contains(err.Error(), "text ready event") {
		t.Fatalf("Run() error = %v, want text ready event error", err)
	}
}

func TestValidateEventRejectsOversizedTextMetadata(t *testing.T) {
	t.Parallel()
	translation := strings.Repeat("x", maxTranslationBytes+1)
	tests := []Event{
		{Type: EventFinal, CaptionID: "caption-1", Translation: &translation},
		{Type: EventError, Stage: strings.Repeat("x", maxStageBytes+1), Message: "failed"},
		{Type: EventSpeech, CaptionID: strings.Repeat("x", maxCaptionIDBytes+1), MIMEType: "audio/wav"},
		{Type: EventSpeech, CaptionID: "caption-1", MIMEType: "audio/wav", AudioBase64: strings.Repeat("A", maxSpeechBase64Bytes+1)},
	}
	for _, event := range tests {
		if err := validateEvent(event); err == nil {
			t.Fatalf("validateEvent(%s) accepted oversized metadata", event.Type)
		}
	}
}

func stringPtr(value string) *string { return &value }
