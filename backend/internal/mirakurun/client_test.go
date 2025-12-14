package mirakurun

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient("http://tuner:40772")
	if client == nil {
		t.Fatal("Failed to create client")
	}
	if client.baseURL != "http://tuner:40772" {
		t.Errorf("Expected baseURL to be http://tuner:40772, got %s", client.baseURL)
	}
}

func TestGetChannels(t *testing.T) {
	client := NewClient("http://tuner:40772")
	
	channels, err := client.GetChannels()
	if err != nil {
		t.Logf("Failed to get channels: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}
	
	if len(channels) > 0 {
		t.Logf("Found %d channels", len(channels))
		for i, ch := range channels {
			if i < 3 { // Log first 3 channels
				t.Logf("Channel %d: %s (ID: %s, Type: %s)", i+1, ch.Name, ch.ID, ch.Type)
			}
		}
	}
}

func TestGetPrograms(t *testing.T) {
	client := NewClient("http://tuner:40772")
	
	// Test with a dummy service ID
	programs, err := client.GetPrograms(1024)
	if err != nil {
		t.Logf("Failed to get programs: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}
	
	t.Logf("Found %d programs", len(programs))
}

func TestGetServiceStream(t *testing.T) {
	client := NewClient("http://tuner:40772")
	
	// Test with a dummy service ID
	stream, err := client.GetServiceStream(1024)
	if err != nil {
		t.Logf("Failed to get service stream: %v", err)
		// This might fail if Mirakurun is not available, which is expected
		return
	}
	defer stream.Close()
	
	t.Log("Successfully connected to service stream")
}