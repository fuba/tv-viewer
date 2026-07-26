package recorder

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const maxResponseSize = 64 * 1024

// Client submits recording reservations to fuba_recorder's yoyaku API.
type Client struct {
	baseURL  string
	http     *http.Client
	mu       sync.Mutex
	reserved map[int64]struct{}
	inflight map[int64]*reservationCall
}

type reservationCall struct {
	done chan struct{}
	err  error
}

func NewClient(baseURL string) *Client {
	return &Client{
		baseURL:  strings.TrimRight(baseURL, "/"),
		http:     &http.Client{Timeout: 5 * time.Second},
		reserved: make(map[int64]struct{}),
		inflight: make(map[int64]*reservationCall),
	}
}

func (c *Client) ReserveProgram(ctx context.Context, programID int64) error {
	if programID <= 0 {
		return fmt.Errorf("invalid program ID %d", programID)
	}
	c.mu.Lock()
	if _, exists := c.reserved[programID]; exists {
		c.mu.Unlock()
		return nil
	}
	if call := c.inflight[programID]; call != nil {
		c.mu.Unlock()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-call.done:
			return call.err
		}
	}
	call := &reservationCall{done: make(chan struct{})}
	c.inflight[programID] = call
	c.mu.Unlock()

	err := c.reserveProgram(ctx, programID)
	c.mu.Lock()
	call.err = err
	if err == nil {
		c.reserved[programID] = struct{}{}
	}
	delete(c.inflight, programID)
	close(call.done)
	c.mu.Unlock()
	return err
}

func (c *Client) reserveProgram(ctx context.Context, programID int64) error {
	query := url.Values{}
	query.Set("program_id", fmt.Sprintf("%d", programID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/record?"+query.Encode(), nil)
	if err != nil {
		return fmt.Errorf("create reservation request: %w", err)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request reservation: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseSize))
	if err != nil {
		return fmt.Errorf("read reservation response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("reservation API returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var result struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		return fmt.Errorf("decode reservation response: %w", err)
	}
	if result.Status != "success" {
		return fmt.Errorf("reservation API returned unexpected status %q", result.Status)
	}
	return nil
}
