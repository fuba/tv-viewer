package mirakurun

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

type Channel struct {
	Type     string    `json:"type"`
	Channel  string    `json:"channel"`
	Name     string    `json:"name"`
	ID       string    `json:"id"`
	Services []Service `json:"services"`
}

type Service struct {
	ID       int    `json:"id"`
	ServiceID int   `json:"serviceId"`
	NetworkID int   `json:"networkId"`
	Name     string `json:"name"`
}

type Program struct {
	ID          int64  `json:"id"`
	EventID     int    `json:"eventId"`
	ServiceID   int    `json:"serviceId"`
	NetworkID   int    `json:"networkId"`
	StartAt     int64  `json:"startAt"`
	Duration    int    `json:"duration"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Genre       Genre  `json:"genre"`
}

type Genre struct {
	Lv1 int `json:"lv1"`
	Lv2 int `json:"lv2"`
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) GetChannels() ([]Channel, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/channels")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var channels []Channel
	if err := json.NewDecoder(resp.Body).Decode(&channels); err != nil {
		return nil, err
	}

	return channels, nil
}

func (c *Client) GetPrograms(serviceID int) ([]Program, error) {
	url := fmt.Sprintf("%s/api/programs?serviceId=%d", c.baseURL, serviceID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var programs []Program
	if err := json.NewDecoder(resp.Body).Decode(&programs); err != nil {
		return nil, err
	}

	return programs, nil
}

func (c *Client) GetServiceStream(serviceID int) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/services/%d/stream", c.baseURL, serviceID)
	resp, err := c.httpClient.Get(url)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (c *Client) GetChannelStream(channelID string) (io.ReadCloser, error) {
	// Default to GR for backward compatibility
	return c.GetChannelStreamWithType("GR", channelID)
}

func (c *Client) GetChannelStreamWithType(channelType, channelID string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/channels/%s/%s/stream", c.baseURL, channelType, channelID)
	log.Printf("Requesting stream from Mirakurun: %s", url)
	
	// Create request with longer timeout for streaming
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	// Use a client with no timeout for streaming
	streamClient := &http.Client{
		Timeout: 0, // No timeout for streaming
	}
	
	resp, err := streamClient.Do(req)
	if err != nil {
		log.Printf("Failed to connect to Mirakurun stream: %v", err)
		return nil, err
	}
	log.Printf("Mirakurun stream response: Status=%d, ContentLength=%d", resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}