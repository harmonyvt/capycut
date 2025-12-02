package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Provider type for LLM backend
type Provider string

const (
	ProviderAzure Provider = "azure"
	ProviderLocal Provider = "local" // OpenAI-compatible (LM Studio, Ollama, etc.)
)

// ClipRequest represents parsed clip parameters from natural language
type ClipRequest struct {
	StartTime string `json:"start_time"` // Format: HH:MM:SS
	EndTime   string `json:"end_time"`   // Format: HH:MM:SS
	Error     string `json:"error,omitempty"`
}

// Parser handles AI-powered natural language parsing
type Parser struct {
	provider   Provider
	endpoint   string
	apiKey     string
	model      string
	apiVersion string // Only used for Azure
	client     *http.Client
}

// Common message type
type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ============================================
// Azure OpenAI Responses API types
// ============================================
type azureRequest struct {
	Model           string    `json:"model"`
	Input           []message `json:"input"`
	MaxOutputTokens int       `json:"max_output_tokens,omitempty"`
}

type azureResponse struct {
	ID                string             `json:"id"`
	Status            string             `json:"status"`
	Output            []azureOutputItem  `json:"output"`
	Error             *apiError          `json:"error,omitempty"`
	IncompleteDetails *incompleteDetails `json:"incomplete_details,omitempty"`
}

type azureOutputItem struct {
	Type    string             `json:"type"`
	Content []azureContentItem `json:"content,omitempty"`
	Text    string             `json:"text,omitempty"`
}

type azureContentItem struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type incompleteDetails struct {
	Reason string `json:"reason"`
}

// ============================================
// OpenAI-compatible API types (LM Studio, Ollama)
// ============================================
type openAIRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
	Temperature float64   `json:"temperature,omitempty"`
}

type openAIResponse struct {
	ID      string         `json:"id"`
	Choices []openAIChoice `json:"choices"`
	Error   *apiError      `json:"error,omitempty"`
}

type openAIChoice struct {
	Message      message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

// NewParser creates a new AI parser, auto-detecting backend from environment
func NewParser() (*Parser, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	// Check for local LLM first (LM Studio, Ollama)
	localEndpoint := os.Getenv("LLM_ENDPOINT")
	localModel := os.Getenv("LLM_MODEL")

	if localEndpoint != "" {
		// Default model for local LLMs
		if localModel == "" {
			localModel = "local-model"
		}

		localEndpoint = strings.TrimSuffix(localEndpoint, "/")

		if debug {
			fmt.Println("\n[DEBUG] Local LLM Configuration:")
			fmt.Printf("  LLM_ENDPOINT: %s\n", localEndpoint)
			fmt.Printf("  LLM_MODEL:    %s\n", localModel)
			fmt.Printf("  API URL:      %s/v1/chat/completions\n", localEndpoint)
			fmt.Println()
		}

		return &Parser{
			provider: ProviderLocal,
			endpoint: localEndpoint,
			model:    localModel,
			client:   &http.Client{Timeout: 120 * time.Second}, // Longer timeout for local models
		}, nil
	}

	// Fall back to Azure OpenAI
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	model := os.Getenv("AZURE_OPENAI_MODEL")
	apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")

	if endpoint == "" {
		return nil, fmt.Errorf("no AI backend configured. Set LLM_ENDPOINT for local LLM or AZURE_OPENAI_ENDPOINT for Azure")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_API_KEY environment variable not set")
	}
	if model == "" {
		return nil, fmt.Errorf("AZURE_OPENAI_MODEL environment variable not set")
	}

	if apiVersion == "" {
		apiVersion = "2025-04-01-preview"
	}

	// Parse and normalize endpoint
	endpoint = strings.TrimSuffix(endpoint, "/")
	parsedURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("invalid AZURE_OPENAI_ENDPOINT URL: %w", err)
	}
	baseURL := fmt.Sprintf("%s://%s", parsedURL.Scheme, parsedURL.Host)

	if debug {
		fmt.Println("\n[DEBUG] Azure OpenAI Configuration:")
		fmt.Printf("  AZURE_OPENAI_ENDPOINT (raw):  %s\n", os.Getenv("AZURE_OPENAI_ENDPOINT"))
		fmt.Printf("  AZURE_OPENAI_ENDPOINT (base): %s\n", baseURL)
		fmt.Printf("  AZURE_OPENAI_API_KEY:         %s...%s\n", apiKey[:4], apiKey[len(apiKey)-4:])
		fmt.Printf("  AZURE_OPENAI_MODEL:           %s\n", model)
		fmt.Printf("  AZURE_OPENAI_API_VERSION:     %s\n", apiVersion)
		fmt.Printf("  API URL:                      %s/openai/responses?api-version=%s\n", baseURL, apiVersion)
		fmt.Println()
	}

	return &Parser{
		provider:   ProviderAzure,
		endpoint:   baseURL,
		apiKey:     apiKey,
		model:      model,
		apiVersion: apiVersion,
		client:     &http.Client{Timeout: 30 * time.Second},
	}, nil
}

// ParseClipRequest parses a natural language clip request into timestamps
func (p *Parser) ParseClipRequest(ctx context.Context, userInput string, videoDuration time.Duration) (*ClipRequest, error) {
	systemPrompt := fmt.Sprintf(`You are a helpful assistant that parses video clipping requests into timestamps.

The video duration is: %s

Your job is to extract start_time and end_time from the user's natural language request.

IMPORTANT RULES:
1. Output times in HH:MM:SS format (e.g., 00:03:00 for 3 minutes)
2. If the user says "first X minutes/seconds", start_time is 00:00:00
3. If the user says "last X minutes/seconds", calculate from the video duration
4. If the user gives a duration from a start point, calculate the end_time
5. Ensure end_time does not exceed the video duration
6. If you cannot understand the request, set an error message

Respond ONLY with valid JSON in this exact format:
{"start_time": "HH:MM:SS", "end_time": "HH:MM:SS"}

Or if there's an error:
{"start_time": "", "end_time": "", "error": "description of the problem"}`, formatDuration(videoDuration))

	switch p.provider {
	case ProviderLocal:
		return p.parseWithOpenAI(ctx, systemPrompt, userInput)
	case ProviderAzure:
		return p.parseWithAzure(ctx, systemPrompt, userInput)
	default:
		return nil, fmt.Errorf("unknown provider: %s", p.provider)
	}
}

// parseWithOpenAI handles OpenAI-compatible APIs (LM Studio, Ollama, etc.)
func (p *Parser) parseWithOpenAI(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	reqBody := openAIRequest{
		Model: p.model,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userInput},
		},
		MaxTokens:   512,
		Temperature: 0.1,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1/chat/completions", p.endpoint)

	if debug {
		fmt.Printf("[DEBUG] Request URL: %s\n", apiURL)
		fmt.Printf("[DEBUG] Request body: %s\n\n", string(jsonBody))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI request failed (is the LLM server running?): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Response status: %s\n", resp.Status)
		fmt.Printf("[DEBUG] Response body: %s\n\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AI request failed: %s\n  URL: %s\n  Response: %s", resp.Status, apiURL, string(body))
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w\nResponse was: %s", err, string(body))
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in AI response")
	}

	content := cleanJSONResponse(apiResp.Choices[0].Message.Content)

	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
	}

	var clipReq ClipRequest
	if err := json.Unmarshal([]byte(content), &clipReq); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w\nResponse was: %s", err, content)
	}

	if clipReq.Error != "" {
		return nil, fmt.Errorf("AI could not parse request: %s", clipReq.Error)
	}

	return &clipReq, nil
}

// parseWithAzure handles Azure OpenAI Responses API
func (p *Parser) parseWithAzure(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	reqBody := azureRequest{
		Model: p.model,
		Input: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userInput},
		},
		MaxOutputTokens: 2048,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/openai/responses?api-version=%s", p.endpoint, p.apiVersion)

	if debug {
		fmt.Printf("[DEBUG] Request URL: %s\n", apiURL)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		if debug {
			fmt.Printf("\n[DEBUG] Request failed:\n")
			fmt.Printf("  URL:      %s\n", apiURL)
			fmt.Printf("  Status:   %s\n", resp.Status)
			fmt.Printf("  Response: %s\n\n", string(body))
		}
		return nil, fmt.Errorf("AI request failed: %s\n  URL: %s\n  Response: %s", resp.Status, apiURL, string(body))
	}

	if debug {
		fmt.Printf("\n[DEBUG] Raw API Response:\n%s\n\n", string(body))
	}

	var apiResp azureResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, fmt.Errorf("failed to parse API response: %w\nResponse was: %s", err, string(body))
	}

	if apiResp.Error != nil {
		return nil, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	content := extractAzureContent(apiResp)
	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
	}
	if content == "" {
		return nil, fmt.Errorf("no content in AI response\nFull response: %s", string(body))
	}

	content = cleanJSONResponse(content)

	var clipReq ClipRequest
	if err := json.Unmarshal([]byte(content), &clipReq); err != nil {
		return nil, fmt.Errorf("failed to parse AI response: %w\nResponse was: %s", err, content)
	}

	if clipReq.Error != "" {
		return nil, fmt.Errorf("AI could not parse request: %s", clipReq.Error)
	}

	return &clipReq, nil
}

// extractAzureContent extracts the text content from Azure API response
func extractAzureContent(resp azureResponse) string {
	for _, output := range resp.Output {
		if output.Type == "message" {
			for _, c := range output.Content {
				if c.Type == "output_text" || c.Type == "text" {
					return c.Text
				}
			}
		}
	}
	return ""
}

// cleanJSONResponse removes markdown code blocks if present
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```json")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}

// formatDuration formats a duration as HH:MM:SS
func formatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

// GetAPIKeyHelp returns help text for setting up AI backend
func GetAPIKeyHelp() string {
	return `To use CapyCut, you need an AI backend configured.

Option 1: Local LLM (FREE - no API key needed!)
  
  LM Studio (recommended):
    1. Download from https://lmstudio.ai
    2. Load a model (e.g., Llama, Mistral, Phi)
    3. Start the local server (default port 1234)
    4. Set: export LLM_ENDPOINT="http://localhost:1234"

  Ollama:
    1. Install from https://ollama.ai
    2. Run: ollama run llama3.2
    3. Set: export LLM_ENDPOINT="http://localhost:11434"
           export LLM_MODEL="llama3.2"

Option 2: Azure OpenAI
  export AZURE_OPENAI_ENDPOINT="https://your-resource.cognitiveservices.azure.com"
  export AZURE_OPENAI_API_KEY="your-api-key"
  export AZURE_OPENAI_MODEL="gpt-4o"

Or create a .env file with these values.`
}

// CheckConfig validates that an AI backend is configured
func CheckConfig() error {
	// Check for local LLM first
	if os.Getenv("LLM_ENDPOINT") != "" {
		return nil // Local LLM configured, no API key needed
	}

	// Check Azure config
	if os.Getenv("AZURE_OPENAI_ENDPOINT") == "" {
		return fmt.Errorf("no AI backend configured")
	}
	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		return fmt.Errorf("AZURE_OPENAI_API_KEY not set")
	}
	if os.Getenv("AZURE_OPENAI_MODEL") == "" {
		return fmt.Errorf("AZURE_OPENAI_MODEL not set")
	}
	return nil
}
