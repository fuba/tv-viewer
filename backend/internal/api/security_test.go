package api

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestWebSocketOriginAllowed(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "https://viewer.example.test,http://puma2:18089,http://puma2:18090,https://tv.example.test")

	tests := []struct {
		name   string
		host   string
		origin string
		want   bool
	}{
		{name: "non browser client", host: "puma2:18089", want: true},
		{name: "same origin", host: "puma2:18089", origin: "http://puma2:18089", want: true},
		{name: "same origin through proxy", host: "puma2:18090", origin: "http://puma2:18090", want: true},
		{name: "different port", host: "puma2:18089", origin: "http://puma2:5173", want: false},
		{name: "configured origin", host: "puma2", origin: "https://tv.example.test", want: true},
		{name: "production HTTPS origin", host: "viewer.example.test", origin: "https://viewer.example.test", want: true},
		{name: "hostname suffix", host: "viewer.example.test", origin: "https://viewer.example.test.attacker.example", want: false},
		{name: "different HTTPS port", host: "viewer.example.test", origin: "https://viewer.example.test:444", want: false},
		{name: "userinfo", host: "viewer.example.test", origin: "https://user@viewer.example.test", want: false},
		{name: "path", host: "viewer.example.test", origin: "https://viewer.example.test/path", want: false},
		{name: "query", host: "viewer.example.test", origin: "https://viewer.example.test?x=1", want: false},
		{name: "fragment", host: "viewer.example.test", origin: "https://viewer.example.test#x", want: false},
		{name: "foreign origin", host: "puma2", origin: "https://attacker.example", want: false},
		{name: "malformed origin", host: "puma2", origin: "://bad", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "http://"+tt.host+"/api/ws/status", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			if got := webSocketOriginAllowed(req); got != tt.want {
				t.Fatalf("webSocketOriginAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestRequestSecurityMiddlewareRejectsCrossSiteMutation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Setenv("ALLOWED_ORIGINS", "http://puma2:18090")
	router := gin.New()
	router.Use(requestSecurityMiddleware())
	router.POST("/api/action", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/api/action", nil)
	req.Header.Set("Origin", "https://attacker.example")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusForbidden)
	}
}

func TestRequestSecurityMiddlewareRequiresJSONBody(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(requestSecurityMiddleware())
	router.POST("/api/action", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodPost, "/api/action", bytes.NewBufferString("value=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, req)
	if response.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusUnsupportedMediaType)
	}
}

func TestWebSocketOriginFailsClosedWithoutAllowlist(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "")
	req := httptest.NewRequest("GET", "http://puma2:18090/api/ws/status", nil)
	req.Header.Set("Origin", "http://puma2:18090")
	if webSocketOriginAllowed(req) {
		t.Fatal("browser origin was accepted without an allowlist")
	}
}

func TestTranslationRequestRequiresAllowedBrowserOrigin(t *testing.T) {
	t.Setenv("ALLOWED_ORIGINS", "https://viewer.example.test")
	tests := []struct {
		name   string
		origin string
		want   bool
	}{
		{name: "missing origin", want: false},
		{name: "allowed origin", origin: "https://viewer.example.test", want: true},
		{name: "foreign origin", origin: "https://attacker.example", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "https://viewer.example.test/api/ws/webrtc/GR_1", nil)
			req.Header.Set("Origin", tt.origin)
			if got := translationRequestAllowed(req); got != tt.want {
				t.Fatalf("translationRequestAllowed() = %v, want %v", got, tt.want)
			}
		})
	}
}
