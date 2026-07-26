package api

import (
	"os"
	"testing"
)

func TestCachedEPGCollapsesRefreshes(t *testing.T) {
	key := "test-cache-collapse"
	oldTTL := os.Getenv("EPG_CACHE_TTL_SECONDS")
	defer os.Setenv("EPG_CACHE_TTL_SECONDS", oldTTL)
	_ = os.Setenv("EPG_CACHE_TTL_SECONDS", "30")

	calls := 0
	fetch := func() (EPGResponse, error) {
		calls++
		return EPGResponse{}, nil
	}
	if _, err := cachedEPG(key, fetch); err != nil {
		t.Fatal(err)
	}
	if _, err := cachedEPG(key, fetch); err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("expected one refresh, got %d", calls)
	}
}
