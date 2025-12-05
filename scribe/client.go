package scribe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// BaseURL is the ElevenLabs API base URL
	BaseURL = "https://api.elevenlabs.io"

	// DefaultTimeout for API requests (long files can take time)
	DefaultTimeout = 10 * time.Minute

	// MaxFileSize is the maximum file size (3GB)
	MaxFileSize = 3 * 1024 * 1024 * 1024

	// MaxSpeakers is the maximum number of speakers for diarization
	MaxSpeakers = 32
)

// Client is the ElevenLabs Scribe API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	debug      bool
}

// ClientOption configures the Client
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL (for testing)
func WithBaseURL(url string) ClientOption {
	return func(c *Client) {
		c.baseURL = strings.TrimSuffix(url, "/")
	}
}

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) ClientOption {
	return func(c *Client) {
		c.httpClient = client
	}
}

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) ClientOption {
	return func(c *Client) {
		c.httpClient.Timeout = timeout
	}
}

// WithDebug enables debug logging
func WithDebug(debug bool) ClientOption {
	return func(c *Client) {
		c.debug = debug
	}
}

// NewClient creates a new ElevenLabs Scribe API client
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	c := &Client{
		apiKey:  apiKey,
		baseURL: BaseURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		debug: false,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// NewClientFromEnv creates a client using the ELEVENLABS_API_KEY environment variable
func NewClientFromEnv(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ELEVENLABS_API_KEY environment variable not set")
	}
	return NewClient(apiKey, opts...)
}

// Transcribe transcribes an audio or video file
func (c *Client) Transcribe(ctx context.Context, req *TranscribeRequest) (*TranscribeResponse, error) {
	if req.FilePath == "" && req.CloudStorageURL == "" {
		return nil, fmt.Errorf("either FilePath or CloudStorageURL must be specified")
	}

	// Validate file exists and check size
	if req.FilePath != "" {
		info, err := os.Stat(req.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to access file: %w", err)
		}
		if info.Size() > MaxFileSize {
			return nil, fmt.Errorf("file size %d exceeds maximum %d bytes (3GB)", info.Size(), MaxFileSize)
		}
	}

	// Validate parameters
	if req.NumSpeakers != nil && (*req.NumSpeakers < 1 || *req.NumSpeakers > MaxSpeakers) {
		return nil, fmt.Errorf("num_speakers must be between 1 and %d", MaxSpeakers)
	}
	if req.DiarizationThreshold != nil && (*req.DiarizationThreshold < 0.1 || *req.DiarizationThreshold > 0.4) {
		return nil, fmt.Errorf("diarization_threshold must be between 0.1 and 0.4")
	}
	if req.Temperature != nil && (*req.Temperature < 0 || *req.Temperature > 2.0) {
		return nil, fmt.Errorf("temperature must be between 0.0 and 2.0")
	}

	// Build multipart form
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add file if specified
	if req.FilePath != "" {
		file, err := os.Open(req.FilePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file: %w", err)
		}
		defer file.Close()

		part, err := writer.CreateFormFile("file", filepath.Base(req.FilePath))
		if err != nil {
			return nil, fmt.Errorf("failed to create form file: %w", err)
		}

		if _, err := io.Copy(part, file); err != nil {
			return nil, fmt.Errorf("failed to copy file to form: %w", err)
		}
	}

	// Add model
	model := req.Model
	if model == "" {
		model = ModelScribeV1
	}
	if err := writer.WriteField("model_id", model); err != nil {
		return nil, fmt.Errorf("failed to write model_id: %w", err)
	}

	// Add cloud storage URL if specified
	if req.CloudStorageURL != "" {
		if err := writer.WriteField("cloud_storage_url", req.CloudStorageURL); err != nil {
			return nil, fmt.Errorf("failed to write cloud_storage_url: %w", err)
		}
	}

	// Add language if specified
	if req.Language != "" {
		if err := writer.WriteField("language_code", req.Language); err != nil {
			return nil, fmt.Errorf("failed to write language_code: %w", err)
		}
	}

	// Add diarization settings
	if req.Diarize != nil {
		if err := writer.WriteField("diarize", strconv.FormatBool(*req.Diarize)); err != nil {
			return nil, fmt.Errorf("failed to write diarize: %w", err)
		}
	}

	if req.NumSpeakers != nil {
		if err := writer.WriteField("num_speakers", strconv.Itoa(*req.NumSpeakers)); err != nil {
			return nil, fmt.Errorf("failed to write num_speakers: %w", err)
		}
	}

	if req.DiarizationThreshold != nil {
		if err := writer.WriteField("diarization_threshold", strconv.FormatFloat(*req.DiarizationThreshold, 'f', 2, 64)); err != nil {
			return nil, fmt.Errorf("failed to write diarization_threshold: %w", err)
		}
	}

	// Add audio events tagging
	if req.TagAudioEvents {
		if err := writer.WriteField("tag_audio_events", "true"); err != nil {
			return nil, fmt.Errorf("failed to write tag_audio_events: %w", err)
		}
	}

	// Add temperature
	if req.Temperature != nil {
		if err := writer.WriteField("temperature", strconv.FormatFloat(*req.Temperature, 'f', 2, 64)); err != nil {
			return nil, fmt.Errorf("failed to write temperature: %w", err)
		}
	}

	// Add webhook settings
	if req.UseWebhook {
		if err := writer.WriteField("webhook", "true"); err != nil {
			return nil, fmt.Errorf("failed to write webhook: %w", err)
		}
	}

	if req.WebhookMetadata != "" {
		if err := writer.WriteField("webhook_metadata", req.WebhookMetadata); err != nil {
			return nil, fmt.Errorf("failed to write webhook_metadata: %w", err)
		}
	}

	// Add logging settings
	if req.EnableLogging != nil {
		if err := writer.WriteField("enable_logging", strconv.FormatBool(*req.EnableLogging)); err != nil {
			return nil, fmt.Errorf("failed to write enable_logging: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	// Create HTTP request
	url := c.baseURL + "/v1/speech-to-text"
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", writer.FormDataContentType())
	httpReq.Header.Set("xi-api-key", c.apiKey)

	if c.debug {
		fmt.Printf("[DEBUG] POST %s\n", url)
		fmt.Printf("[DEBUG] Content-Type: %s\n", writer.FormDataContentType())
	}

	// Execute request
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.debug {
		fmt.Printf("[DEBUG] Response status: %d\n", resp.StatusCode)
		if len(respBody) < 2000 {
			fmt.Printf("[DEBUG] Response body: %s\n", string(respBody))
		} else {
			fmt.Printf("[DEBUG] Response body (truncated): %s...\n", string(respBody[:2000]))
		}
	}

	// Handle errors
	if resp.StatusCode != http.StatusOK {
		var apiErr APIError
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		apiErr.StatusCode = resp.StatusCode
		return nil, &apiErr
	}

	// Parse response
	var result TranscribeResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// TranscribeWithRetry transcribes with automatic retry on transient failures
func (c *Client) TranscribeWithRetry(ctx context.Context, req *TranscribeRequest, maxRetries int) (*TranscribeResponse, error) {
	var lastErr error

	backoff := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := c.Transcribe(ctx, req)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if apiErr, ok := err.(*APIError); ok {
			// Don't retry client errors (4xx)
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 {
				return nil, err
			}
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		// Wait before retry
		if attempt < maxRetries {
			wait := backoff[attempt]
			if attempt >= len(backoff) {
				wait = backoff[len(backoff)-1]
			}

			if c.debug {
				fmt.Printf("[DEBUG] Retry %d/%d after %v: %v\n", attempt+1, maxRetries, wait, err)
			}

			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}
		}
	}

	return nil, fmt.Errorf("transcription failed after %d retries: %w", maxRetries, lastErr)
}

// ParseTranscript converts the API response into a high-level Transcript structure
func (c *Client) ParseTranscript(resp *TranscribeResponse) *Transcript {
	return ParseResponse(resp)
}

// GetAPIKeyHelp returns help text for setting up the API key
func GetAPIKeyHelp() string {
	return `To use the ElevenLabs Scribe API, you need an API key.

1. Sign up at https://elevenlabs.io
2. Go to Profile Settings → API Keys
3. Create a new API key
4. Set the environment variable:

   export ELEVENLABS_API_KEY="your-api-key"

Or create a .env file with:
   ELEVENLABS_API_KEY=your-api-key

Pricing: https://elevenlabs.io/pricing
- Free tier includes limited transcription minutes
- Pay-as-you-go pricing for additional usage`
}
