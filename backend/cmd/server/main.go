package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/fuba/tv-viewer/internal/api"
	"github.com/fuba/tv-viewer/internal/db"
	"github.com/gin-gonic/gin"
)

func main() {
	if os.Getenv(gin.EnvGinMode) == "" {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize database
	database, err := db.Init("tv-viewer.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// Setup Gin router
	router := gin.Default()
	if err := router.SetTrustedProxies([]string{"127.0.0.1", "::1"}); err != nil {
		log.Fatal("Failed to configure trusted proxies:", err)
	}

	// Setup API routes
	api.SetupRoutes(router, database)

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "18088"
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		log.Fatal("PORT must be an integer between 1 and 65535")
	}
	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = "0.0.0.0"
	}
	if net.ParseIP(bindAddress) == nil {
		log.Fatal("BIND_ADDRESS must be an IP address")
	}

	log.Printf("Starting server on configured address")
	server := &http.Server{
		Addr:              net.JoinHostPort(bindAddress, strconv.Itoa(portNumber)),
		Handler:           router,
		ReadTimeout:       15 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    32 << 10,
	}
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal("Failed to start server:", err)
	}
}
