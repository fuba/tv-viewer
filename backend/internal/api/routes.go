package api

import (
	"database/sql"
	"fmt"
	"io"
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
		
		// Stream selection
		api.GET("/channels/:id/stream-info", getStreamInfo(db))
		api.POST("/channels/:id/select-streams", selectStreams(db))
		
		// CS channel service-specific streaming
		api.GET("/channels/CS/:id/service/:serviceId/stream", streamCSService(db))
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
		
		// Find the channel type and first service ID for CS channels
		var channelType string
		var firstServiceID int64
		var isCSChannel bool
		
		log.Printf("Looking for channel %s in %d channels", channelID, len(channels))
		
		for _, ch := range channels {
			log.Printf("Checking channel: %s (type: %s) against %s", ch.Channel, ch.Type, channelID)
			if ch.Channel == channelID {
				channelType = ch.Type
				isCSChannel = channelType == "CS"
				log.Printf("Found matching channel %s, type: %s, isCS: %v, services: %d", 
					channelID, channelType, isCSChannel, len(ch.Services))
				
				// For CS channels, get the first service ID
				if isCSChannel && len(ch.Services) > 0 {
					firstServiceID = ch.Services[0].ID
					log.Printf("CS channel %s has %d services, using service ID %d (%s)", 
						channelID, len(ch.Services), firstServiceID, ch.Services[0].Name)
				}
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
		
		var stream io.ReadCloser
		if isCSChannel && firstServiceID != 0 {
			// For CS channels, use service-specific streaming
			stream, err = client.GetServiceStream(firstServiceID)
			if err != nil {
				log.Printf("Failed to get stream for CS channel %s service %d: %v", channelID, firstServiceID, err)
				// Fall back to channel stream
				stream, err = client.GetChannelStreamWithType(channelType, channelID)
			}
		} else {
			// For non-CS channels, use regular channel streaming
			stream, err = client.GetChannelStreamWithType(channelType, channelID)
		}
		
		if err != nil {
			log.Printf("Failed to get stream for channel %s (type %s): %v", channelID, channelType, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get channel stream",
				"details": err.Error(),
			})
			return
		}
		// Note: Do NOT defer stream.Close() here as the encoder needs to manage the stream
		
		// Build the stream URL for ffprobe
		streamURL := fmt.Sprintf("%s/api/channels/%s/stream/%s", mirakurunURL, channelType, url.QueryEscape(channelID))
		
		// Start encoding with stream URL for potential stream analysis
		session, err := encoderInstance.StartEncodingWithStreamSelection(channelID, stream, streamURL, -1, -1)
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

func getStreamInfo(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid channel ID encoding",
			})
			return
		}
		
		// Get session info from encoder
		info, err := encoderInstance.GetSessionInfo(channelID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
			return
		}
		
		c.JSON(http.StatusOK, info)
	}
}

func selectStreams(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid channel ID encoding",
			})
			return
		}
		
		var req struct {
			VideoStreamIndex int `json:"video_stream_index"`
			AudioStreamIndex int `json:"audio_stream_index"`
		}
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
			return
		}
		
		// Update stream selection
		if err := encoderInstance.UpdateStreamSelection(channelID, req.VideoStreamIndex, req.AudioStreamIndex); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": err.Error(),
			})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{
			"status": "updated",
			"video_stream_index": req.VideoStreamIndex,
			"audio_stream_index": req.AudioStreamIndex,
		})
	}
}

func streamCSService(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID, err := url.QueryUnescape(c.Param("id"))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid channel ID encoding",
			})
			return
		}
		
		serviceIDStr := c.Param("serviceId")
		serviceID, err := strconv.ParseInt(serviceIDStr, 10, 64)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid service ID",
			})
			return
		}
		
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		client := mirakurun.NewClient(mirakurunURL)
		
		log.Printf("CS Service streaming: channel=%s, serviceID=%d", channelID, serviceID)
		
		// Direct service streaming
		stream, err := client.GetServiceStream(serviceID)
		if err != nil {
			log.Printf("Failed to get service stream %d for CS channel %s: %v", serviceID, channelID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get service stream",
				"details": err.Error(),
			})
			return
		}
		
		// Build service-specific stream URL
		streamURL := fmt.Sprintf("%s/api/services/%d/stream", mirakurunURL, serviceID)
		
		// Start encoding with service-specific naming
		sessionID := fmt.Sprintf("CS%s_S%d", channelID, serviceID)
		session, err := encoderInstance.StartEncodingWithStreamSelection(sessionID, stream, streamURL, -1, -1)
		if err != nil {
			stream.Close()
			log.Printf("Failed to start encoding for CS service %d: %v", serviceID, err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to start encoding",
				"details": err.Error(),
			})
			return
		}
		
		c.JSON(http.StatusOK, gin.H{
			"status": "streaming",
			"sessionId": session.ID,
			"serviceId": serviceID,
			"playlistUrl": fmt.Sprintf("/api/stream/%s/playlist.m3u8", sessionID),
		})
	}
}