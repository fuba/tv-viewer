package main

import (
	"log"
	"os"

	"github.com/fuba/tv-viewer/internal/api"
	"github.com/fuba/tv-viewer/internal/db"
	"github.com/gin-gonic/gin"
)

func main() {
	// Initialize database
	database, err := db.Init("tv-viewer.db")
	if err != nil {
		log.Fatal("Failed to initialize database:", err)
	}
	defer database.Close()

	// Setup Gin router
	router := gin.Default()

	// Setup API routes
	api.SetupRoutes(router, database)

	// Get port from environment or use default
	port := os.Getenv("PORT")
	if port == "" {
		port = "18088"
	}

	log.Printf("Starting server on port %s", port)
	if err := router.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
