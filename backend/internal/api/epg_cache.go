package api

import (
	"os"
	"strconv"
	"sync"
	"time"
)

type epgCacheEntry struct {
	response  EPGResponse
	expiresAt time.Time
}

type epgCacheStore struct {
	mu      sync.Mutex
	entries map[string]epgCacheEntry
}

var epgCache = epgCacheStore{entries: make(map[string]epgCacheEntry)}

func epgCacheTTL() time.Duration {
	seconds := 30
	if value, err := strconv.Atoi(os.Getenv("EPG_CACHE_TTL_SECONDS")); err == nil && value >= 0 {
		seconds = value
	}
	return time.Duration(seconds) * time.Second
}

func cachedEPG(key string, fetch func() (EPGResponse, error)) (EPGResponse, error) {
	ttl := epgCacheTTL()
	epCache := &epgCache
	epCache.mu.Lock()
	if entry, ok := epCache.entries[key]; ok && time.Now().Before(entry.expiresAt) {
		epCache.mu.Unlock()
		return entry.response, nil
	}
	// Keep the lock while fetching to collapse concurrent refreshes into one request.
	defer epCache.mu.Unlock()
	response, err := fetch()
	if err != nil {
		return EPGResponse{}, err
	}
	epCache.entries[key] = epgCacheEntry{response: response, expiresAt: time.Now().Add(ttl)}
	return response, nil
}
