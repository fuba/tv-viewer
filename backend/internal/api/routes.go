package api

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/fuba/tv-viewer/internal/encoder"
	"github.com/fuba/tv-viewer/internal/mirakurun"
	"github.com/fuba/tv-viewer/internal/programguide"
	"github.com/fuba/tv-viewer/internal/recorder"
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
		// The UI is served through local reverse proxies that rewrite Host and
		// may use a different port from the backend. Access control is handled
		// by the network boundary, so signaling accepts those browser origins.
		CheckOrigin: func(_ *http.Request) bool { return true },
	}
)

func SetupRoutes(router *gin.Engine, db *sql.DB) {
	// Initialize WebSocket hub for session monitoring
	wsHub = wsmonitor.NewHub(encoderInstance)
	go wsHub.Run()
	log.Println("[wsmonitor] WebSocket hub started")

	router.Use(requestConcurrencyMiddleware())

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
			mirakurunURL := os.Getenv("MIRAKURUN_URL")
			if mirakurunURL == "" {
				mirakurunURL = "http://tuner:40772"
			}
			mirakurunOK := mirakurun.NewClient(mirakurunURL).Health()
			programURL := os.Getenv("PROGRAM_API_URL")
			if programURL == "" {
				programURL = "http://puma2:40870"
			}
			programOK := programguide.NewClient(programURL).Health()
			status := "ok"
			code := http.StatusOK
			if !mirakurunOK || !programOK {
				status = "degraded"
				code = http.StatusServiceUnavailable
			}
			c.JSON(code, gin.H{
				"status":       status,
				"dependencies": gin.H{"mirakurun": mirakurunOK, "programGuide": programOK, "nativePipeline": true},
			})
		})

		// Get active encoding sessions
		api.GET("/sessions", func(c *gin.Context) {
			sessions := encoderInstance.GetActiveSessions()
			c.JSON(http.StatusOK, gin.H{
				"active_sessions": sessions,
				"count":           len(sessions),
			})
		})

		// Stop all encoding sessions
		api.POST("/sessions/stop-all", func(c *gin.Context) {
			encoderInstance.StopAllSessions()
			c.JSON(http.StatusOK, gin.H{
				"status": "stopped",
			})
		})

		// Get native pipeline logs for a channel
		api.GET("/logs/:channel", func(c *gin.Context) {
			channelID := c.Param("channel")
			logs := encoderInstance.GetChannelLogs(channelID)
			c.JSON(http.StatusOK, gin.H{
				"channel": channelID,
				"logs":    logs,
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
				"status":        "updated",
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
		recorderURL := os.Getenv("FUBA_RECORDER_API_URL")
		if recorderURL == "" {
			recorderURL = "http://127.0.0.1:37569"
		}
		api.POST("/recordings/reserve", reserveRecording(recorder.NewClient(recorderURL)))

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
		// Get query parameters
		channelType := c.Query("type") // GR, BS, CS (optional)
		hoursStr := c.DefaultQuery("hours", "24")
		hours, err := strconv.Atoi(hoursStr)
		if err != nil || hours < 1 || hours > 168 {
			hours = 24
		}

		cacheKey := fmt.Sprintf("%s:%d", channelType, hours)
		response, err := cachedEPG(cacheKey, func() (EPGResponse, error) {
			return fetchEPG(channelType, hours)
		})
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch program guide", "details": err.Error()})
			return
		}
		c.JSON(http.StatusOK, response)
	}
}

func fetchEPG(channelType string, hours int) (EPGResponse, error) {
	now := time.Now()
	from := now.Add(-time.Hour)
	to := now.Add(time.Duration(hours) * time.Hour)
	programURL := os.Getenv("PROGRAM_API_URL")
	client := programguide.NewClient(programURL)
	services, err := client.Services()
	if err != nil {
		return EPGResponse{}, fmt.Errorf("fetch services: %w", err)
	}
	programs, err := client.Search(from, to, channelType)
	if err != nil {
		return EPGResponse{}, fmt.Errorf("search programs: %w", err)
	}
	programsByService := make(map[int][]mirakurun.Program)
	for _, p := range programs {
		programsByService[p.ServiceID] = append(programsByService[p.ServiceID], mirakurun.Program{
			ID: p.ID, EventID: p.EventID, ServiceID: p.ServiceID, StartAt: p.StartAt, Duration: p.Duration,
			Name: p.Name, Description: p.Description,
			Genre: mirakurun.Genre{Lv1: p.Genre.Lv1, Lv2: p.Genre.Lv2},
		})
	}
	var result []EPGChannel
	for _, service := range services {
		if channelType != "" && service.ChannelType != channelType {
			continue
		}
		servicePrograms := programsByService[service.ServiceID]
		if len(servicePrograms) == 0 {
			continue
		}
		result = append(result, EPGChannel{
			Channel: service.ChannelNumber, Type: service.ChannelType, ServiceID: service.ID,
			Name: service.Name, Programs: servicePrograms,
		})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Type != result[j].Type {
			order := map[string]int{"GR": 0, "BS": 1, "CS": 2}
			return order[result[i].Type] < order[result[j].Type]
		}
		return result[i].Name < result[j].Name
	})
	return EPGResponse{Channels: result, TimeRange: struct {
		From int64 `json:"from"`
		To   int64 `json:"to"`
	}{From: from.UnixMilli(), To: to.UnixMilli()}}, nil
}

// TunerStatus represents the status of tuners by type
type TunerStatus struct {
	Type         string `json:"type"`
	Total        int    `json:"total"`
	Using        int    `json:"using"`
	Free         int    `json:"free"`
	ViewerUsing  int    `json:"viewerUsing"`
	EPGUsing     int    `json:"epgUsing"`
	OtherUsing   int    `json:"otherUsing"`
	ViewerShared int    `json:"viewerShared"`
}

func summarizeTuners(tuners []mirakurun.Tuner) []TunerStatus {
	typeMap := make(map[string]*TunerStatus)
	for _, tuner := range tuners {
		typeKey := strings.Join(tuner.Types, "/")
		if _, ok := typeMap[typeKey]; !ok {
			typeMap[typeKey] = &TunerStatus{Type: typeKey}
		}
		status := typeMap[typeKey]
		status.Total++
		if tuner.IsUsing {
			status.Using++
		}
		if tuner.IsFree {
			status.Free++
		}
		if !tuner.IsUsing {
			continue
		}

		viewer, epg, other := false, false, false
		for _, user := range tuner.Users {
			switch {
			case strings.HasPrefix(user.Agent, "tv-viewer/"):
				viewer = true
			case strings.HasPrefix(user.ID, "Mirakurun:getEPG()"):
				epg = true
			default:
				other = true
			}
		}
		if len(tuner.Users) == 0 {
			other = true
		}
		if viewer {
			status.ViewerUsing++
		}
		if epg {
			status.EPGUsing++
		}
		if other {
			status.OtherUsing++
		}
		if viewer && (epg || other) {
			status.ViewerShared++
		}
	}

	result := make([]TunerStatus, 0, len(typeMap))
	for _, status := range typeMap {
		result = append(result, *status)
	}
	sort.Slice(result, func(i, j int) bool {
		order := map[string]int{"GR": 0, "BS/CS": 1}
		return order[result[i].Type] < order[result[j].Type]
	})
	return result
}

func getTuners() gin.HandlerFunc {
	return func(c *gin.Context) {
		mirakurunURL := os.Getenv("MIRAKURUN_URL")
		if mirakurunURL == "" {
			mirakurunURL = "http://tuner:40772"
		}

		tuners, err := mirakurun.NewClient(mirakurunURL).GetTuners()
		if err != nil {
			log.Printf("Failed to fetch tuners: %v", err)
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to fetch tuners"})
			return
		}

		c.JSON(http.StatusOK, summarizeTuners(tuners))
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
