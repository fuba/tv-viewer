package voicetranslate

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	maxEventSize         = 6 << 20
	maxAudioFrameSize    = 65536
	writeTimeout         = 5 * time.Second
	maxCaptionIDBytes    = 256
	maxTranslationBytes  = 4096
	maxSourceTextBytes   = 4096
	maxStageBytes        = 64
	maxMessageBytes      = 512
	maxLanguageBytes     = 32
	maxMetadataBytes     = 128
	maxSpeechWAVBytes    = 8*96000*2*2 + 64<<10
	maxSpeechBase64Bytes = (maxSpeechWAVBytes + 2) / 3 * 4
)

const (
	EventReady           = "ready"
	EventPartial         = "partial"
	EventFinal           = "final"
	EventSpeech          = "speech"
	EventSpeechCancelled = "speech_cancelled"
	EventError           = "error"
)

var languagePattern = regexp.MustCompile(`^[a-z]{2,3}(?:-[A-Za-z]{2,8})?$`)

// Config contains the server-side VoiceTranslate connection settings.
type Config struct {
	URL            string
	Origin         string
	AccessToken    string
	SourceLanguage string
	TargetLanguage string
	SpeechEnabled  bool
}

// ConfigFromEnv reads the shared credential without exposing it to the browser.
func ConfigFromEnv() (Config, error) {
	config := Config{
		URL:            strings.TrimSpace(os.Getenv("VOICETRANSLATE_URL")),
		Origin:         strings.TrimSpace(os.Getenv("VOICETRANSLATE_ORIGIN")),
		AccessToken:    strings.TrimSpace(os.Getenv("VOICETRANSLATE_ACCESS_TOKEN")),
		SourceLanguage: "auto",
		TargetLanguage: "ja",
		SpeechEnabled:  true,
	}
	if config.URL == "" {
		return Config{}, errors.New("VOICETRANSLATE_URL is required")
	}
	if config.Origin == "" {
		return Config{}, errors.New("VOICETRANSLATE_ORIGIN is required")
	}
	if tokenFile := strings.TrimSpace(os.Getenv("VOICETRANSLATE_TOKEN_FILE")); tokenFile != "" {
		if config.AccessToken != "" {
			return Config{}, errors.New("set only one of VOICETRANSLATE_TOKEN_FILE and VOICETRANSLATE_ACCESS_TOKEN")
		}
		data, err := os.ReadFile(tokenFile)
		if err != nil {
			return Config{}, fmt.Errorf("read VoiceTranslate token file: %w", err)
		}
		if len(data) > 512 {
			return Config{}, errors.New("VoiceTranslate token file is unexpectedly large")
		}
		config.AccessToken = strings.TrimSpace(string(data))
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

// Validate rejects credential and transport configurations that violate the API contract.
func (c Config) Validate() error {
	endpoint, err := url.Parse(c.URL)
	if err != nil || endpoint.Host == "" || endpoint.Path == "" || endpoint.User != nil || endpoint.RawQuery != "" || endpoint.Fragment != "" {
		return errors.New("invalid VoiceTranslate WebSocket URL")
	}
	if endpoint.Scheme != "wss" {
		host := endpoint.Hostname()
		if endpoint.Scheme != "ws" || (host != "localhost" && net.ParseIP(host) == nil) {
			return errors.New("VoiceTranslate WebSocket URL must use wss")
		}
		ip := net.ParseIP(host)
		if ip != nil && !ip.IsLoopback() {
			return errors.New("insecure VoiceTranslate WebSocket URL is allowed only on loopback")
		}
	}
	origin, err := url.Parse(c.Origin)
	if err != nil || origin.Scheme == "" || origin.Host == "" || origin.Path != "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" {
		return errors.New("invalid VoiceTranslate Origin")
	}
	if origin.Scheme != "https" {
		host := origin.Hostname()
		ip := net.ParseIP(host)
		if origin.Scheme != "http" || (host != "localhost" && (ip == nil || !ip.IsLoopback())) {
			return errors.New("VoiceTranslate Origin must use https")
		}
	}
	if len(c.AccessToken) < 32 || len(c.AccessToken) > 256 {
		return errors.New("VoiceTranslate access token must contain 32 to 256 characters")
	}
	if c.SourceLanguage != "auto" && !languagePattern.MatchString(c.SourceLanguage) {
		return errors.New("invalid VoiceTranslate source language")
	}
	if c.TargetLanguage == "auto" || !languagePattern.MatchString(c.TargetLanguage) {
		return errors.New("invalid VoiceTranslate target language")
	}
	if c.SpeechEnabled && c.TargetLanguage != "ja" {
		return errors.New("VoiceTranslate speech replacement currently requires target language ja")
	}
	return nil
}

type startMessage struct {
	Type           string `json:"type"`
	SourceLanguage string `json:"source_language"`
	TargetLanguage string `json:"target_language"`
	AccessToken    string `json:"access_token"`
	SpeechEnabled  bool   `json:"speech_enabled"`
}

// Event is a validated envelope from the VoiceTranslate gateway.
type Event struct {
	Type            string  `json:"type"`
	CaptionID       string  `json:"caption_id,omitempty"`
	Text            string  `json:"text,omitempty"`
	Translation     *string `json:"translation,omitempty"`
	StableText      string  `json:"stable_text,omitempty"`
	UnstableText    string  `json:"unstable_text,omitempty"`
	SourceLanguage  string  `json:"source_language,omitempty"`
	TargetLanguage  string  `json:"target_language,omitempty"`
	AudioBase64     string  `json:"audio_base64,omitempty"`
	MIMEType        string  `json:"mime_type,omitempty"`
	Speaker         string  `json:"speaker,omitempty"`
	DurationSeconds float64 `json:"duration_seconds,omitempty"`
	Stage           string  `json:"stage,omitempty"`
	Message         string  `json:"message,omitempty"`
	SampleRate      int     `json:"sample_rate,omitempty"`
	Channels        int     `json:"channels,omitempty"`
	Format          string  `json:"format,omitempty"`
}

// Client streams already-paced PCM frames and receives asynchronous translation events.
type Client struct {
	config Config
	dialer *websocket.Dialer
}

func NewClient(config Config) *Client {
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	return &Client{config: config, dialer: &dialer}
}

func (c *Client) Run(ctx context.Context, audio <-chan []byte, events chan<- Event) error {
	if err := c.config.Validate(); err != nil {
		return err
	}
	header := http.Header{}
	header.Set("Origin", c.config.Origin)
	conn, response, err := c.dialer.DialContext(ctx, c.config.URL, header)
	if err != nil {
		if response != nil {
			return fmt.Errorf("connect VoiceTranslate: HTTP %d", response.StatusCode)
		}
		return fmt.Errorf("connect VoiceTranslate: %w", err)
	}
	defer conn.Close()
	conn.SetReadLimit(maxEventSize)
	watcherDone := make(chan struct{})
	defer close(watcherDone)
	go func() {
		select {
		case <-ctx.Done():
			_ = conn.Close()
		case <-watcherDone:
		}
	}()

	start := startMessage{
		Type: "start", SourceLanguage: c.config.SourceLanguage,
		TargetLanguage: c.config.TargetLanguage, AccessToken: c.config.AccessToken,
		SpeechEnabled: c.config.SpeechEnabled,
	}
	if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
		return err
	}
	if err := conn.WriteJSON(start); err != nil {
		return fmt.Errorf("start VoiceTranslate: %w", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(6 * time.Second)); err != nil {
		return err
	}
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		return fmt.Errorf("read VoiceTranslate ready event: %w", err)
	}
	if messageType != websocket.TextMessage {
		return errors.New("VoiceTranslate sent an unexpected non-text ready event")
	}
	var ready Event
	if err := json.Unmarshal(payload, &ready); err != nil {
		return fmt.Errorf("decode VoiceTranslate ready event: %w", err)
	}
	if err := validateEvent(ready); err != nil {
		return fmt.Errorf("validate VoiceTranslate ready event: %w", err)
	}
	if ready.Type != EventReady || ready.SampleRate != 16000 || ready.Channels != 1 || ready.Format != "pcm_s16le" {
		return errors.New("VoiceTranslate sent an invalid ready contract")
	}
	if err := sendEvent(ctx, events, ready); err != nil {
		return err
	}
	if err := conn.SetReadDeadline(time.Time{}); err != nil {
		return err
	}

	readResult := make(chan error, 1)
	go func() {
		readResult <- readEvents(ctx, conn, events)
	}()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-readResult:
			return err
		case frame, ok := <-audio:
			if !ok {
				return nil
			}
			if len(frame) == 0 {
				continue
			}
			if len(frame)%2 != 0 || len(frame) > maxAudioFrameSize {
				return fmt.Errorf("invalid VoiceTranslate PCM frame size %d", len(frame))
			}
			if err := conn.SetWriteDeadline(time.Now().Add(writeTimeout)); err != nil {
				return err
			}
			if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				return fmt.Errorf("send VoiceTranslate PCM: %w", err)
			}
		}
	}
}

func readEvents(ctx context.Context, conn *websocket.Conn, events chan<- Event) error {
	for {
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			return fmt.Errorf("read VoiceTranslate event: %w", err)
		}
		if messageType != websocket.TextMessage {
			return errors.New("VoiceTranslate sent an unexpected binary event")
		}
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return fmt.Errorf("decode VoiceTranslate event: %w", err)
		}
		if len(envelope.Type) > 32 {
			return errors.New("VoiceTranslate event type exceeds 32 bytes")
		}
		switch envelope.Type {
		case EventPartial, EventFinal, EventSpeech, EventSpeechCancelled, EventError:
		default:
			continue
		}
		var event Event
		if err := json.Unmarshal(payload, &event); err != nil {
			return fmt.Errorf("decode VoiceTranslate event: %w", err)
		}
		if err := validateEvent(event); err != nil {
			return fmt.Errorf("validate VoiceTranslate %s event: %w", event.Type, err)
		}
		if err := sendEvent(ctx, events, event); err != nil {
			return err
		}
	}
}

func validateEvent(event Event) error {
	fields := []struct {
		name    string
		value   string
		maximum int
	}{
		{name: "type", value: event.Type, maximum: 32},
		{name: "caption_id", value: event.CaptionID, maximum: maxCaptionIDBytes},
		{name: "text", value: event.Text, maximum: maxSourceTextBytes},
		{name: "stable_text", value: event.StableText, maximum: maxSourceTextBytes},
		{name: "unstable_text", value: event.UnstableText, maximum: maxSourceTextBytes},
		{name: "source_language", value: event.SourceLanguage, maximum: maxLanguageBytes},
		{name: "target_language", value: event.TargetLanguage, maximum: maxLanguageBytes},
		{name: "mime_type", value: event.MIMEType, maximum: maxMetadataBytes},
		{name: "speaker", value: event.Speaker, maximum: maxMetadataBytes},
		{name: "stage", value: event.Stage, maximum: maxStageBytes},
		{name: "message", value: event.Message, maximum: maxMessageBytes},
		{name: "format", value: event.Format, maximum: maxMetadataBytes},
	}
	if event.Translation != nil {
		fields = append(fields, struct {
			name    string
			value   string
			maximum int
		}{name: "translation", value: *event.Translation, maximum: maxTranslationBytes})
	}
	for _, field := range fields {
		if len(field.value) > field.maximum {
			return fmt.Errorf("VoiceTranslate field %s exceeds %d bytes", field.name, field.maximum)
		}
	}
	if len(event.AudioBase64) > maxSpeechBase64Bytes {
		return errors.New("VoiceTranslate audio_base64 exceeds the 8-second speech limit")
	}
	if event.Type != EventSpeech && event.AudioBase64 != "" {
		return fmt.Errorf("VoiceTranslate %s event unexpectedly contains audio", event.Type)
	}
	return nil
}

func sendEvent(ctx context.Context, events chan<- Event, event Event) error {
	select {
	case events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
