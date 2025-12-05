//go:build realtime

package scribe

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// RealtimeWebSocketURL is the ElevenLabs Scribe v2 realtime WebSocket endpoint
	RealtimeWebSocketURL = "wss://api.elevenlabs.io/v1/speech-to-text/realtime"

	// DefaultSampleRate for realtime transcription
	DefaultSampleRate = 16000

	// DefaultEncoding for audio input
	DefaultEncoding = "pcm_s16le"
)

// RealtimeClient handles WebSocket-based realtime transcription
type RealtimeClient struct {
	apiKey    string
	config    *RealtimeConfig
	conn      *websocket.Conn
	mu        sync.Mutex
	connected bool
	closed    bool
	debug     bool
}

// RealtimeTranscript represents a realtime transcription result
type RealtimeTranscript struct {
	// Text is the transcribed text
	Text string `json:"text"`

	// IsFinal indicates if this is a final (non-interim) result
	IsFinal bool `json:"is_final"`

	// Confidence is the model's confidence (0-1)
	Confidence float64 `json:"confidence,omitempty"`

	// Words contains word-level details if available
	Words []RealtimeWord `json:"words,omitempty"`

	// Language is the detected language
	Language string `json:"language,omitempty"`
}

// RealtimeWord represents a word in realtime transcription
type RealtimeWord struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// RealtimeMessage types for WebSocket communication
type realtimeInitMessage struct {
	Type              string             `json:"type"`
	ModelID           string             `json:"model_id,omitempty"`
	Language          string             `json:"language,omitempty"`
	SampleRate        int                `json:"sample_rate,omitempty"`
	Encoding          string             `json:"encoding,omitempty"`
	EndpointingConfig *endpointingConfig `json:"endpointing_config,omitempty"`
}

type endpointingConfig struct {
	Type             string `json:"type"`
	MinEndpointingMs int    `json:"min_endpointing_ms,omitempty"`
	MaxEndpointingMs int    `json:"max_endpointing_ms,omitempty"`
}

type realtimeAudioMessage struct {
	Type  string `json:"type"`
	Audio string `json:"audio"` // Base64 encoded audio
}

type realtimeResponse struct {
	Type       string              `json:"type"`
	Transcript *RealtimeTranscript `json:"transcript,omitempty"`
	Error      *APIError           `json:"error,omitempty"`
	Message    string              `json:"message,omitempty"`
}

// NewRealtimeClient creates a new realtime transcription client
func NewRealtimeClient(apiKey string, config *RealtimeConfig, debug bool) (*RealtimeClient, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	if config == nil {
		config = &RealtimeConfig{
			Model:      ModelScribeV2Realtime,
			SampleRate: DefaultSampleRate,
			Encoding:   DefaultEncoding,
		}
	}

	if config.Model == "" {
		config.Model = ModelScribeV2Realtime
	}
	if config.SampleRate == 0 {
		config.SampleRate = DefaultSampleRate
	}
	if config.Encoding == "" {
		config.Encoding = DefaultEncoding
	}

	return &RealtimeClient{
		apiKey: apiKey,
		config: config,
		debug:  debug,
	}, nil
}

// NewRealtimeClientFromEnv creates a realtime client using environment variables
func NewRealtimeClientFromEnv(config *RealtimeConfig, debug bool) (*RealtimeClient, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY environment variable not set")
	}
	return NewRealtimeClient(apiKey, config, debug)
}

// Connect establishes the WebSocket connection
func (c *RealtimeClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return fmt.Errorf("already connected")
	}

	// Build WebSocket URL with API key
	url := fmt.Sprintf("%s?xi-api-key=%s", RealtimeWebSocketURL, c.apiKey)

	if c.debug {
		fmt.Printf("[DEBUG] Connecting to WebSocket: %s\n", RealtimeWebSocketURL)
	}

	// Create dialer with custom headers
	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
	}

	// Connect
	header := http.Header{}
	conn, resp, err := dialer.DialContext(ctx, url, header)
	if err != nil {
		if resp != nil {
			return fmt.Errorf("WebSocket connection failed (status %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("WebSocket connection failed: %w", err)
	}

	c.conn = conn
	c.connected = true

	// Send initialization message
	initMsg := realtimeInitMessage{
		Type:       "init",
		ModelID:    c.config.Model,
		Language:   c.config.Language,
		SampleRate: c.config.SampleRate,
		Encoding:   c.config.Encoding,
	}

	if c.debug {
		fmt.Printf("[DEBUG] Sending init message: %+v\n", initMsg)
	}

	if err := c.conn.WriteJSON(initMsg); err != nil {
		c.Close()
		return fmt.Errorf("failed to send init message: %w", err)
	}

	// Start reading responses
	go c.readResponses()

	return nil
}

// readResponses handles incoming WebSocket messages
func (c *RealtimeClient) readResponses() {
	defer func() {
		c.mu.Lock()
		c.connected = false
		c.mu.Unlock()
	}()

	for {
		// Get connection reference while holding lock
		c.mu.Lock()
		if c.closed || c.conn == nil {
			c.mu.Unlock()
			return
		}
		// Keep a reference to conn and check closed state atomically
		conn := c.conn
		closed := c.closed
		c.mu.Unlock()

		if closed {
			return
		}

		_, message, err := conn.ReadMessage()
		if err != nil {
			c.mu.Lock()
			isClosed := c.closed
			onError := c.config.OnError
			c.mu.Unlock()

			if onError != nil && !isClosed {
				onError(fmt.Errorf("WebSocket read error: %w", err))
			}
			return
		}

		if c.debug {
			fmt.Printf("[DEBUG] Received: %s\n", string(message))
		}

		var resp realtimeResponse
		if err := json.Unmarshal(message, &resp); err != nil {
			if c.config.OnError != nil {
				c.config.OnError(fmt.Errorf("failed to parse response: %w", err))
			}
			continue
		}

		switch resp.Type {
		case "transcript":
			if c.config.OnTranscript != nil && resp.Transcript != nil {
				c.config.OnTranscript(resp.Transcript.Text, resp.Transcript.IsFinal)
			}
		case "error":
			if c.config.OnError != nil && resp.Error != nil {
				c.config.OnError(resp.Error)
			}
		case "connected":
			if c.debug {
				fmt.Println("[DEBUG] WebSocket connected successfully")
			}
		}
	}
}

// SendAudio sends audio data for transcription
// The audio should be in the format specified during initialization
func (c *RealtimeClient) SendAudio(audio []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := realtimeAudioMessage{
		Type:  "audio",
		Audio: base64.StdEncoding.EncodeToString(audio),
	}

	return c.conn.WriteJSON(msg)
}

// SendAudioChunk sends a chunk of audio data (convenience method)
func (c *RealtimeClient) SendAudioChunk(audio []byte, chunkSize int) error {
	for i := 0; i < len(audio); i += chunkSize {
		end := i + chunkSize
		if end > len(audio) {
			end = len(audio)
		}

		if err := c.SendAudio(audio[i:end]); err != nil {
			return err
		}

		// Small delay to avoid overwhelming the connection
		time.Sleep(20 * time.Millisecond)
	}
	return nil
}

// StreamFile streams audio from a file for realtime transcription
func (c *RealtimeClient) StreamFile(ctx context.Context, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	return c.StreamReader(ctx, file)
}

// StreamReader streams audio from an io.Reader for realtime transcription
func (c *RealtimeClient) StreamReader(ctx context.Context, reader io.Reader) error {
	// Calculate chunk size and duration based on actual sample rate
	// For 16-bit audio (2 bytes per sample), mono channel
	const bytesPerSample = 2
	const chunkDurationMs = 100 // 100ms chunks

	sampleRate := c.config.SampleRate
	if sampleRate == 0 {
		sampleRate = DefaultSampleRate
	}

	// Calculate bytes for the chunk duration: (sampleRate * bytesPerSample * ms) / 1000
	chunkSize := (sampleRate * bytesPerSample * chunkDurationMs) / 1000
	buffer := make([]byte, chunkSize)

	// Calculate actual chunk duration for pacing
	chunkDuration := time.Duration(chunkDurationMs) * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := reader.Read(buffer)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read error: %w", err)
		}

		if n > 0 {
			if err := c.SendAudio(buffer[:n]); err != nil {
				return fmt.Errorf("send error: %w", err)
			}

			// Pace the sending based on actual audio duration of bytes read
			// This accounts for partial reads
			actualDuration := time.Duration(n) * time.Second / time.Duration(sampleRate*bytesPerSample)
			if actualDuration > 0 {
				time.Sleep(actualDuration)
			}
		} else {
			// If no bytes read but no EOF, use default chunk duration
			time.Sleep(chunkDuration)
		}
	}
}

// Flush signals the end of audio input
func (c *RealtimeClient) Flush() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.connected || c.conn == nil {
		return fmt.Errorf("not connected")
	}

	msg := map[string]string{"type": "flush"}
	return c.conn.WriteJSON(msg)
}

// Close closes the WebSocket connection
func (c *RealtimeClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	c.connected = false

	if c.conn != nil {
		err := c.conn.Close()
		c.conn = nil
		return err
	}

	return nil
}

// IsConnected returns whether the client is connected
func (c *RealtimeClient) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// TranscribeRealtimeFromFile is a convenience function to transcribe a file using realtime API
// Returns the full transcript when complete
func TranscribeRealtimeFromFile(ctx context.Context, apiKey string, filePath string, language string) (string, error) {
	var result strings.Builder
	var mu sync.Mutex
	done := make(chan error, 1)

	config := &RealtimeConfig{
		Model:      ModelScribeV2Realtime,
		Language:   language,
		SampleRate: DefaultSampleRate,
		Encoding:   DefaultEncoding,
		OnTranscript: func(text string, isFinal bool) {
			if isFinal {
				mu.Lock()
				result.WriteString(text)
				result.WriteString(" ")
				mu.Unlock()
			}
		},
		OnError: func(err error) {
			select {
			case done <- err:
			default:
			}
		},
	}

	client, err := NewRealtimeClient(apiKey, config, false)
	if err != nil {
		return "", err
	}
	defer client.Close()

	if err := client.Connect(ctx); err != nil {
		return "", err
	}

	// Stream the file
	go func() {
		err := client.StreamFile(ctx, filePath)
		if err != nil {
			done <- err
			return
		}
		client.Flush()
		// Wait a bit for final transcripts
		time.Sleep(2 * time.Second)
		done <- nil
	}()

	// Wait for completion or error
	select {
	case err := <-done:
		if err != nil {
			return "", err
		}
	case <-ctx.Done():
		return "", ctx.Err()
	}

	mu.Lock()
	defer mu.Unlock()
	return strings.TrimSpace(result.String()), nil
}
