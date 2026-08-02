package webrtc

import "testing"

func TestSharedStreamRemembersLatestTranslationStatus(t *testing.T) {
	t.Parallel()
	stream := NewSharedStream()
	stream.rememberTranslationStatus([]byte(`{"type":"translation-status","status":"ready"}`))
	stream.rememberTranslationStatus([]byte(`{"type":"translation-status","status":"error"}`))
	if got := string(stream.translationStatusSnapshot()); got != `{"type":"translation-status","status":"error"}` {
		t.Fatalf("translation status = %s", got)
	}
	stream.rememberTranslationStatus([]byte(`{"type":"translation-status","status":"speaking"}`))
	stream.rememberTranslationStatus([]byte(`{"type":"show","text":"ARIB"}`))
	if got := string(stream.translationStatusSnapshot()); got != `{"type":"translation-status","status":"error"}` {
		t.Fatalf("caption replaced translation status: %s", got)
	}
}
