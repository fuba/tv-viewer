package mirakurun

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
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
	ID        int64  `json:"id"`
	ServiceID int    `json:"serviceId"`
	NetworkID int    `json:"networkId"`
	Name      string `json:"name"`
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
	timeout := 5 * time.Second
	if value, err := strconv.Atoi(os.Getenv("HTTP_REQUEST_TIMEOUT_SECONDS")); err == nil && value > 0 {
		timeout = time.Duration(value) * time.Second
	}
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) GetChannels() ([]Channel, error) {
	return c.GetChannelsContext(context.Background())
}

func (c *Client) GetChannelsContext(ctx context.Context) ([]Channel, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/channels", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
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

	var allPrograms []Program
	if err := json.NewDecoder(resp.Body).Decode(&allPrograms); err != nil {
		return nil, err
	}

	// Filter programs to include only current and future programs
	now := time.Now().UnixMilli()
	var programs []Program
	for _, p := range allPrograms {
		// Include programs that are currently airing or will air in the future
		if p.StartAt+int64(p.Duration) > now {
			programs = append(programs, p)
		}
	}

	// Sort programs by start time
	for i := 0; i < len(programs)-1; i++ {
		for j := i + 1; j < len(programs); j++ {
			if programs[i].StartAt > programs[j].StartAt {
				programs[i], programs[j] = programs[j], programs[i]
			}
		}
	}

	return programs, nil
}

// GetAllPrograms fetches all programs from Mirakurun without serviceId filter
func (c *Client) GetAllPrograms() ([]Program, error) {
	url := fmt.Sprintf("%s/api/programs", c.baseURL)
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

func (c *Client) GetServiceStream(serviceID int64) (io.ReadCloser, error) {
	return c.GetServiceStreamContext(context.Background(), serviceID)
}

func (c *Client) GetServiceStreamContext(ctx context.Context, serviceID int64) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/services/%d/stream", c.baseURL, serviceID)
	log.Printf("Requesting service stream from Mirakurun: %s", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tv-viewer/1.0")
	streamClient := newStreamClient()

	resp, err := streamClient.Do(req)
	if err != nil {
		log.Printf("Failed to connect to Mirakurun service stream: %v", err)
		return nil, err
	}
	log.Printf("Mirakurun service stream response: Status=%d, ContentLength=%d", resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("unexpected status code %d and failed to close response: %w", resp.StatusCode, closeErr)
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func (c *Client) GetChannelStream(channelID string) (io.ReadCloser, error) {
	// Default to GR for backward compatibility
	return c.GetChannelStreamWithType("GR", channelID)
}

func (c *Client) GetChannelStreamWithType(channelType, channelID string) (io.ReadCloser, error) {
	return c.GetChannelStreamWithTypeContext(context.Background(), channelType, channelID)
}

func (c *Client) GetChannelStreamWithTypeContext(ctx context.Context, channelType, channelID string) (io.ReadCloser, error) {
	url := fmt.Sprintf("%s/api/channels/%s/%s/stream", c.baseURL, channelType, channelID)
	log.Printf("Requesting stream from Mirakurun: %s", url)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "tv-viewer/1.0")
	streamClient := newStreamClient()

	resp, err := streamClient.Do(req)
	if err != nil {
		log.Printf("Failed to connect to Mirakurun stream: %v", err)
		return nil, err
	}
	log.Printf("Mirakurun stream response: Status=%d, ContentLength=%d", resp.StatusCode, resp.ContentLength)

	if resp.StatusCode != http.StatusOK {
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, fmt.Errorf("unexpected status code %d and failed to close response: %w", resp.StatusCode, closeErr)
		}
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	return resp.Body, nil
}

func streamConnectTimeout() time.Duration {
	seconds := 10
	if value, err := strconv.Atoi(os.Getenv("STREAM_CONNECT_TIMEOUT_SECONDS")); err == nil && value > 0 {
		seconds = value
	}
	return time.Duration(seconds) * time.Second
}

// newStreamClient bounds connection and response-header setup without
// applying a deadline to the live response body.
func newStreamClient() *http.Client {
	timeout := streamConnectTimeout()
	dialer := &net.Dialer{Timeout: timeout, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		DialContext:           dialer.DialContext,
		ResponseHeaderTimeout: timeout,
		IdleConnTimeout:       90 * time.Second,
	}
	return &http.Client{Transport: transport, Timeout: 0}
}

type Tuner struct {
	Types   []string    `json:"types"`
	IsUsing bool        `json:"isUsing"`
	IsFree  bool        `json:"isFree"`
	Users   []TunerUser `json:"users"`
}

type TunerUser struct {
	ID    string `json:"id"`
	Agent string `json:"agent"`
	URL   string `json:"url"`
}

func (c *Client) GetTuners() ([]Tuner, error) {
	resp, err := c.httpClient.Get(c.baseURL + "/api/tuners")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	var tuners []Tuner
	if err := json.NewDecoder(resp.Body).Decode(&tuners); err != nil {
		return nil, err
	}
	return tuners, nil
}

func (c *Client) Health() bool {
	_, err := c.GetChannels()
	return err == nil
}
