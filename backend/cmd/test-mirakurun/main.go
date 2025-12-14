package main

import (
	"fmt"
	"log"

	"github.com/fuba/tv-viewer/internal/mirakurun"
)

func main() {
	client := mirakurun.NewClient("http://tuner:40772")
	
	fmt.Println("Testing Mirakurun connection...")
	
	// Test channels
	channels, err := client.GetChannels()
	if err != nil {
		log.Fatal("Failed to get channels:", err)
	}
	
	fmt.Printf("Found %d channels\n", len(channels))
	if len(channels) > 0 {
		fmt.Printf("First channel: %s (ID: %s)\n", channels[0].Name, channels[0].ID)
		
		// Test programs for first channel
		if len(channels[0].Services) > 0 {
			serviceID := channels[0].Services[0].ID
			programs, err := client.GetPrograms(serviceID)
			if err != nil {
				log.Printf("Failed to get programs: %v", err)
			} else {
				fmt.Printf("Found %d programs for service %d\n", len(programs), serviceID)
			}
		}
	}
}