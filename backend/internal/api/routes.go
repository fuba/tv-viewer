package api

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/wsmonitor"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

var (
	encoderInstance = encoder.New()
	wsHub           *wsmonitor.Hub
	wsUpgrader      = websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
		CheckOrigin: func(r *http.Request) bool {
			return true // Allow all origins for development
		},
	}
)

func SetupRoutes(router *gin.Engine, db *sql.DB) {
	// Initialize WebSocket hub for session monitoring
	wsHub = wsmonitor.NewHub(encoderInstance)
	go wsHub.Run()
	log.Println("[wsmonitor] WebSocket hub started")

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

		// Programs
		api.GET("/programs", getPrograms(db))

		// EPG (Electronic Program Guide) - all channels program grid
		api.GET("/epg", getEPG(db))

		
		// Stream selection (for debugging - shows current encoder stream info)
		api.GET("/channels/:id/stream-info", getStreamInfo(db))

		// Tuner status
		api.GET("/tuners", getTuners())

		// WebSocket endpoint for session monitoring
		api.GET("/ws/session/:sessionId", handleWebSocket)

		// WebSocket status (for debugging)
		api.GET("/ws/status", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{
				"connected_sessions": wsHub.GetConnectedSessionCount(),
				"sessions":           wsHub.GetConnectedSessions(),
			})
		})

		// WebRTC routes
		SetupWebRTCRoutes(api)
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

// EPGChannel represents a channel with its programs for EPG display
type EPGChannel struct {
	Channel   string              `json:"channel"`
	Type      string              `json:"type"`
	ServiceID int64               `json:"serviceId"`
	Name      string              `json:"name"`
	Programs  []mirakurun.Program `json:"programs"`
}

// EPGResponse represents the EPG API response
type EPGResponse struct {
	Channels  []EPGChannel `json:"channels"`
	TimeRange struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	} `json:"timeRange"`
}

func getEPG(db *sql.DB) gin.HandlerFunc {
	return func(c *gin.Context) {
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}

		client := mirakurun.NewClient(mirakurunURL)

		// Get query parameters
		channelType := c.Query("type") // GR, BS, CS (optional)
		hoursStr := c.DefaultQuery("hours", "24")
		hours, err := strconv.Atoi(hoursStr)
		if err != nil || hours < 1 || hours > 168 {
			hours = 24
		}

		// Calculate time range
		now := time.Now()
		fromTime := now.Add(-1 * time.Hour).UnixMilli() // Start 1 hour before now
		toTime := now.Add(time.Duration(hours) * time.Hour).UnixMilli()

		// Get all channels
		channels, err := client.GetChannels()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch channels",
			})
			return
		}

		// Get all programs
		allPrograms, err := client.GetAllPrograms()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to fetch programs",
			})
			return
		}

		// Group programs by serviceId
		programsByService := make(map[int][]mirakurun.Program)
		for _, p := range allPrograms {
			// Filter by time range
			programEnd := p.StartAt + int64(p.Duration)
			if programEnd < fromTime || p.StartAt > toTime {
				continue
			}
			programsByService[p.ServiceID] = append(programsByService[p.ServiceID], p)
		}

		// Sort programs within each service by start time
		for serviceID := range programsByService {
			sort.Slice(programsByService[serviceID], func(i, j int) bool {
				return programsByService[serviceID][i].StartAt < programsByService[serviceID][j].StartAt
			})
		}

		// Build EPG response
		var epgChannels []EPGChannel
		for _, ch := range channels {
			// Filter by channel type if specified
			if channelType != "" && ch.Type != channelType {
				continue
			}

			// Process each service in the channel
			for _, svc := range ch.Services {
				programs := programsByService[svc.ServiceID]
				if len(programs) == 0 {
					continue // Skip channels with no programs
				}

				epgChannels = append(epgChannels, EPGChannel{
					Channel:   ch.Channel,
					Type:      ch.Type,
					ServiceID: svc.ID,
					Name:      svc.Name,
					Programs:  programs,
				})
			}
		}

		// Sort channels by type and name
		sort.Slice(epgChannels, func(i, j int) bool {
			if epgChannels[i].Type != epgChannels[j].Type {
				typeOrder := map[string]int{"GR": 0, "BS": 1, "CS": 2}
				return typeOrder[epgChannels[i].Type] < typeOrder[epgChannels[j].Type]
			}
			return epgChannels[i].Name < epgChannels[j].Name
		})

		response := EPGResponse{
			Channels: epgChannels,
		}
		response.TimeRange.From = fromTime
		response.TimeRange.To = toTime

		c.JSON(http.StatusOK, response)
	}
}

// TunerStatus represents the status of tuners by type
type TunerStatus struct {
	Type  string `json:"type"`
	Total int    `json:"total"`
	Using int    `json:"using"`
	Free  int    `json:"free"`
}

func getTuners() gin.HandlerFunc {
	return func(c *gin.Context) {
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}

		resp, err := http.Get(mirakurunURL + "/api/tuners")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tuners"})
			return
		}
		defer resp.Body.Close()

		var tuners []struct {
			Types   []string `json:"types"`
			IsUsing bool     `json:"isUsing"`
			IsFree  bool     `json:"isFree"`
		}

		if err := json.NewDecoder(resp.Body).Decode(&tuners); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to parse tuners"})
			return
		}

		// Group by type
		typeMap := make(map[string]*TunerStatus)
		for _, t := range tuners {
			// Create a key from types (e.g., "GR" or "BS/CS")
			typeKey := ""
			for i, tp := range t.Types {
				if i > 0 {
					typeKey += "/"
				}
				typeKey += tp
			}

			if _, ok := typeMap[typeKey]; !ok {
				typeMap[typeKey] = &TunerStatus{Type: typeKey}
			}
			typeMap[typeKey].Total++
			if t.IsUsing {
				typeMap[typeKey].Using++
			}
			if t.IsFree {
				typeMap[typeKey].Free++
			}
		}

		// Convert to slice
		result := make([]TunerStatus, 0, len(typeMap))
		for _, v := range typeMap {
			result = append(result, *v)
		}

		c.JSON(http.StatusOK, result)
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

// handleWebSocket handles WebSocket connections for session monitoring
func handleWebSocket(c *gin.Context) {
	sessionID, err := url.QueryUnescape(c.Param("sessionId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid session ID"})
		return
	}

	conn, err := wsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("[wsmonitor] WebSocket upgrade failed for session %s: %v", sessionID, err)
		return
	}

	client := wsmonitor.NewClient(wsHub, conn, sessionID)
	wsHub.Register(client)

	// Start read/write pumps in goroutines
	go client.WritePump()
	go client.ReadPump()
}