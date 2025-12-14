package api

import (
	"database/sql"
	"net/http"
	"os"

	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/gin-gonic/gin"
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
		// TODO: Implement channel streaming
		c.JSON(http.StatusOK, gin.H{
			"status": "streaming",
		})
	}
}

func getPrograms(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		serviceID := c.Query("serviceId")
		if serviceID == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "serviceId is required",
			})
			return
		}
		
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}
		
		// TODO: Convert serviceID string to int and fetch programs
		c.JSON(http.StatusOK, []gin.H{})
	}
}

func getPlaylist(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement HLS playlist generation
		c.String(http.StatusOK, "#EXTM3U\n")
	}
}

func getSegment(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: Implement segment serving
		c.Status(http.StatusNotFound)
	}
}