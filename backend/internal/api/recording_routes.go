package api

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
)

type recordingReserver interface {
	ReserveProgram(context.Context, int64) error
}

func reserveRecording(reserver recordingReserver) gin.HandlerFunc {
	return func(c *gin.Context) {
		if !sameHostOrigin(c.Request) {
			c.JSON(http.StatusForbidden, gin.H{"error": "cross-site reservation request rejected"})
			return
		}
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1024)
		var request struct {
			ProgramID int64 `json:"programId"`
		}
		if err := c.ShouldBindJSON(&request); err != nil || request.ProgramID <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "valid programId is required"})
			return
		}
		if err := reserver.ReserveProgram(c.Request.Context(), request.ProgramID); err != nil {
			log.Printf("[Recording] Failed to reserve program %d: %v", request.ProgramID, err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "fuba_recorder rejected the reservation"})
			return
		}
		log.Printf("[Recording] Reserved program %d with fuba_recorder", request.ProgramID)
		c.JSON(http.StatusCreated, gin.H{"status": "reserved", "programId": request.ProgramID})
	}
}

func sameHostOrigin(request *http.Request) bool {
	origin := request.Header.Get("Origin")
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Hostname() == "" {
		return false
	}
	host := request.Host
	if hostname, _, err := net.SplitHostPort(host); err == nil {
		host = hostname
	}
	host = strings.Trim(host, "[]")
	return strings.EqualFold(parsed.Hostname(), host)
}
