package programguide

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"time"
)

type Client struct {
	baseURL string
	http    *http.Client
}

type Service struct {
	ID               int64  `json:"id"`
	ServiceID        int    `json:"serviceId"`
	Name             string `json:"name"`
	ChannelType      string `json:"channelType"`
	ChannelNumber    string `json:"channelNumber"`
	RemoteControlKey int    `json:"remoteControlKeyId"`
}

type Program struct {
	ID          int64  `json:"id"`
	EventID     int    `json:"eventId"`
	ServiceID   int    `json:"serviceId"`
	StartAt     int64  `json:"startAt"`
	Duration    int    `json:"duration"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Genre       struct {
		Lv1 int `json:"lv1"`
		Lv2 int `json:"lv2"`
	} `json:"genre"`
}

func NewClient(baseURL string) *Client {
	if baseURL == "" {
		baseURL = "http://puma2:40870"
	}
	timeout := 5 * time.Second
	if seconds, err := strconv.Atoi(os.Getenv("HTTP_REQUEST_TIMEOUT_SECONDS")); err == nil && seconds > 0 {
		timeout = time.Duration(seconds) * time.Second
	}
	return &Client{baseURL: baseURL, http: &http.Client{Timeout: timeout}}
}

func (c *Client) get(path string, query url.Values, result any) error {
	u := c.baseURL + path
	if len(query) > 0 {
		u += "?" + query.Encode()
	}
	resp, err := c.http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("program guide returned status %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(result)
}

func (c *Client) Services() ([]Service, error) {
	var services []Service
	if err := c.get("/services", nil, &services); err != nil {
		return nil, err
	}
	return services, nil
}

func (c *Client) Search(from, to time.Time, channelType string) ([]Program, error) {
	query := url.Values{}
	query.Set("startFrom", strconv.FormatInt(from.UnixMilli(), 10))
	query.Set("startTo", strconv.FormatInt(to.UnixMilli(), 10))
	if channelType != "" {
		query.Set("channelType", channelType)
	}
	var programs []Program
	if err := c.get("/search", query, &programs); err != nil {
		return nil, err
	}
	return programs, nil
}

func (c *Client) Health() bool {
	_, err := c.Services()
	return err == nil
}
