package api

import (
	"database/sql"
	"io"
	"net/http"
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

	// Static files
	router.Static("/", "./frontend/dist")
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
		channelID := c.Param("id")
		
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		client := mirakurun.NewClient(mirakurunURL)
		stream, err := client.GetChannelStream(channelID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to get channel stream",
			})
			return
		}
		defer stream.Close()
		
		// Start encoding
		session, err := encoderInstance.StartEncoding(channelID, stream)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to start encoding",
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
		channelID := c.Param("channel")
		playlistPath := encoderInstance.GetPlaylistPath(channelID)
		
		// Check if playlist exists
		if _, err := os.Stat(playlistPath); os.IsNotExist(err) {
			c.Status(http.StatusNotFound)
			return
		}
		
		c.File(playlistPath)
	}
}

func getSubtitles(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		channelID := c.Param("channel")
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
		channelID := c.Param("channel")
		segment := c.Param("segment")
		
		segmentPath := filepath.Join("stream", channelID, segment)
		
		// Check if segment exists
		if _, err := os.Stat(segmentPath); os.IsNotExist(err) {
			c.Status(http.StatusNotFound)
			return
		}
		
		c.File(segmentPath)
	}
}