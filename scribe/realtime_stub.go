//go:build !realtime

package scribe

import (
	"context"
	"fmt"
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
// Note: This is a stub. Build with -tags=realtime and github.com/gorilla/websocket
// to enable full realtime functionality.
type RealtimeClient struct {
	apiKey string
	config *RealtimeConfig
	debug  bool
}

// RealtimeTranscript represents a realtime transcription result
type RealtimeTranscript struct {
	Text       string         `json:"text"`
	IsFinal    bool           `json:"is_final"`
	Confidence float64        `json:"confidence,omitempty"`
	Words      []RealtimeWord `json:"words,omitempty"`
	Language   string         `json:"language,omitempty"`
}

// RealtimeWord represents a word in realtime transcription
type RealtimeWord struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

// ErrRealtimeNotAvailable is returned when realtime features are used without the build tag
var ErrRealtimeNotAvailable = fmt.Errorf("realtime transcription not available: build with -tags=realtime and add github.com/gorilla/websocket dependency")

// NewRealtimeClient creates a new realtime transcription client
// Note: This is a stub. Build with -tags=realtime to enable.
func NewRealtimeClient(apiKey string, config *RealtimeConfig, debug bool) (*RealtimeClient, error) {
	return nil, ErrRealtimeNotAvailable
}

// NewRealtimeClientFromEnv creates a realtime client using environment variables
// Note: This is a stub. Build with -tags=realtime to enable.
func NewRealtimeClientFromEnv(config *RealtimeConfig, debug bool) (*RealtimeClient, error) {
	return nil, ErrRealtimeNotAvailable
}

// Connect establishes the WebSocket connection
func (c *RealtimeClient) Connect(ctx context.Context) error {
	return ErrRealtimeNotAvailable
}

// SendAudio sends audio data for transcription
func (c *RealtimeClient) SendAudio(audio []byte) error {
	return ErrRealtimeNotAvailable
}

// StreamFile streams audio from a file for realtime transcription
func (c *RealtimeClient) StreamFile(ctx context.Context, filePath string) error {
	return ErrRealtimeNotAvailable
}

// Flush signals the end of audio input
func (c *RealtimeClient) Flush() error {
	return ErrRealtimeNotAvailable
}

// Close closes the WebSocket connection
func (c *RealtimeClient) Close() error {
	return nil
}

// IsConnected returns whether the client is connected
func (c *RealtimeClient) IsConnected() bool {
	return false
}

// TranscribeRealtimeFromFile transcribes a file using the realtime API
// Note: This is a stub. Build with -tags=realtime to enable.
func TranscribeRealtimeFromFile(ctx context.Context, apiKey string, filePath string, language string) (string, error) {
	return "", ErrRealtimeNotAvailable
}
