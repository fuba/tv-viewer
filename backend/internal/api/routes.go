package api

import (
	"database/sql"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/gin-gonic/gin"
)

var (
	encoderInstance = encoder.New()
)

func SetupRoutes(router *gin.Engine, db *sql.DB) {
	// CORS middleware
	router.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	})

	// API routes
	api := router.Group("/api")
	{
		// Health check
		api.GET("/health", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"status": "ok",
			})
		})

		// Get active encoding sessions
		api.GET("/sessions", func(c *gin.Context) {
			sessions := encoderInstance.GetActiveSessions()
			c.JSON(http.StatusOK, gin.H{
				"active_sessions": sessions,
				"count": len(sessions),
			})
		})

		// Stop all encoding sessions
		api.POST("/sessions/stop-all", func(c *gin.Context) {
			encoderInstance.StopAllSessions()
			c.JSON(http.StatusOK, gin.H{
				"status": "stopped",
			})
		})

		// Get FFmpeg logs for a channel
		api.GET("/logs/:channel", func(c *gin.Context) {
			channelID := c.Param("channel")
			logs := encoderInstance.GetChannelLogs(channelID)
			c.JSON(http.StatusOK, gin.H{
				"channel": channelID,
				"logs": logs,
			})
		})

		// Get NVENC status
		api.GET("/nvenc/status", func(c *gin.Context) {
			status := encoderInstance.GetNVENCStatus()
			c.JSON(http.StatusOK, status)
		})

		// Toggle NVENC usage
		api.POST("/nvenc/toggle", func(c *gin.Context) {
			var req struct {
				Enabled bool `json:"enabled"`
			}
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
				return
			}
			
			if err := encoderInstance.SetUseNVENC(req.Enabled); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
				return
			}
			
			c.JSON(http.StatusOK, gin.H{
				"status": "updated",
				"nvenc_enabled": req.Enabled,
			})
		})

		// Settings
		api.GET("/settings", getSettings(db))
		api.PUT("/settings", updateSettings(db))

		// Channels
		api.GET("/channels", getChannels(db))
		api.GET("/channels/:id/stream", streamChannel(db))

		// Programs
		api.GET("/programs", getPrograms(db))

		// Streaming
		api.GET("/stream/:channel/playlist.m3u8", getPlaylist(db))
		api.GET("/stream/:channel/subtitles.ass", getSubtitles(db))
		api.GET("/stream/:channel/:segment", getSegment(db))
	}

	// Health check for root
	router.GET("/", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"message": "TV Viewer API Server",
			"version": "1.0.0",
		})
	})
}

func getSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement settings retrieval
		c.JSON(http.StatusOK, gin.H{
			"mirakurun_url": "http://tuner:40772",
		})
	}
}

func updateSettings(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement settings update
		c.JSON(http.StatusOK, gin.H{
			"status": "updated",
		})
	}
}

func getChannels(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		client := mirakurun.NewClient(mirakurunURL)
		channels, err := client.GetChannels()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch channels",
			})
			return
		}
		
		c.JSON(http.StatusOK, channels)
	}
}

func streamChannel(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid channel ID encoding",
			})
			return
		}
		
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		// First, get channel info to determine type
		client := mirakurun.NewClient(mirakurunURL)
		channels, err := client.GetChannels()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get channels",
			})
			return
		}
		
		// Find the channel type
		var channelType string
		for _, ch := range channels {
			if ch.Channel == channelID {
				channelType = ch.Type
				break
			}
		}
		
		if channelType == "" {
			log.Printf("Channel %s not found in channels list", channelID)
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Channel not found",
			})
			return
		}
		
		log.Printf("Streaming channel %s with type %s", channelID, channelType)
		stream, err := client.GetChannelStreamWithType(channelType, channelID)
		if err != nil {
			log.Printf("Failed to get stream for channel %s (type %s): %v", channelID, channelType, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get channel stream",
				"details": err.Error(),
			})
			return
		}
		// Note: Do NOT defer stream.Close() here as the encoder needs to manage the stream
		
		// Start encoding
		session, err := encoderInstance.StartEncoding(channelID, stream)
		if err != nil {
			stream.Close() // Only close on error
			log.Printf("Failed to start encoding for channel %s: %v", channelID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to start encoding",
				"details": err.Error(),
			})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{
			"status": "streaming",
			"sessionId": session.ID,
			"playlistUrl": "/api/stream/" + channelID + "/playlist.m3u8",
		})
	}
}

func getPrograms(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceIDStr := c.Query("serviceId")
		if serviceIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "serviceId is required",
			})
			return
		}
		
		serviceID, err := strconv.Atoi(serviceIDStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid serviceId",
			})
			return
		}
		
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		client := mirakurun.NewClient(mirakurunURL)
		programs, err := client.GetPrograms(serviceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch programs",
			})
			return
		}
		
		c.JSON(http.StatusOK, programs)
	}
}

func getPlaylist(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("channel"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		playlistPath := encoderInstance.GetPlaylistPath(channelID)
		log.Printf("Looking for playlist at: %s", playlistPath)
		
		// Check if playlist exists
		if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
			log.Printf("Playlist not found: %s (error: %v)", playlistPath, err)
			c.Status(http.StatusNotFound)
			return
		}
		
		c.File(playlistPath)
	}
}

func getSubtitles(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("channel"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		subtitlePath := encoderInstance.GetSubtitlePath(channelID)
		
		// Check if subtitle file exists
		if _, err := os.Stat(subtitlePath); os.IsNotExist(err) {
			// Return empty ASS file if no subtitles
			c.Header("Content-Type", "text/plain; charset=utf-8")
			c.String(http.StatusOK, "[Script Info]\nTitle: Empty\n\n[V4+ Styles]\n\n[Events]\n")
			return
		}
		
		c.File(subtitlePath)
	}
}

func getSegment(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("channel"))
		if err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		segment := c.Param("segment")
		
		segmentPath := filepath.Join("stream", channelID, segment)
		
		// Debug: log the requested path
		log.Printf("Serving segment: %s (channel: %s, segment: %s)", segmentPath, channelID, segment)
		
		// Check if segment exists
		if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
			log.Printf("Segment not found: %s", segmentPath)
			c.Status(http.StatusNotFound)
			return
		}
		
		// Set proper content type for TS files
		c.Header("Content-Type", "video/mp2t")
		c.File(segmentPath)
	}
}