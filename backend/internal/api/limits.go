package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

var (
	errViewerLimit  = errors.New("viewer limit reached")
	errChannelLimit = errors.New("channel session limit reached")
)

func configuredLimit(name string, fallback int) int {
	if value, err := strconv.Atoi(os.Getenv(name)); err == nil && value > 0 {
		return value
	}
	return fallback
}

func maxViewers() int {
	return configuredLimit("MAX_VIEWERS", 4)
}

func requestConcurrencyMiddleware() gin.HandlerFunc {
	limit := configuredLimit("MAX_HTTP_CONCURRENCY", 32)
	semaphore := make(chan struct{}, limit)
	return func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/api/ws/") {
			c.Next()
			return
		}
		select {
		case semaphore <- struct{}{}:
			defer func() { <-semaphore }()
			c.Next()
		default:
			c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{"error": "server is busy"})
		}
	}
}

func (r *sharedSessionRegistry) canAccept(channelID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if existing := r.sessions[channelID]; existing != nil {
		if existing.stream.SubscriberCount() >= maxViewers() {
			return errViewerLimit
		}
		return nil
	}
	// One tuner/encoder pipeline is shared by every viewer. A different channel
	// must never start while the active channel exists.
	if len(r.sessions) >= 1 {
		return errChannelLimit
	}
	return nil
}
