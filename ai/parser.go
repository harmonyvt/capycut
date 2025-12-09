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

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// Provider type for LLM backend
type Provider string

const (
	ProviderAzure          Provider = "azure"
	ProviderLocal          Provider = "local"           // OpenAI-compatible (LM Studio, Ollama, etc.)
	ProviderAzureAnthropic Provider = "azure_anthropic" // Azure Anthropic (Claude via Azure)
)

// ClipRequest represents parsed clip parameters from natural language
type ClipRequest struct {
	StartTime string `json:"start_time"` // Format: HH:MM:SS
	EndTime   string `json:"end_time"`   // Format: HH:MM:SS
	Error     string `json:"error,omitempty"`
}

// Parser handles AI-powered natural language parsing
type Parser struct {
	provider        Provider
	endpoint        string
	apiKey          string
	model           string
	apiVersion      string // Only used for Azure
	client          *http.Client
	anthropicClient *anthropic.Client // For Azure Anthropic
}

// ParserProgressStatus represents the current status of parsing
type ParserProgressStatus int

const (
	ParserStatusIdle ParserProgressStatus = iota
	ParserStatusConnecting
	ParserStatusSendingRequest
	ParserStatusWaitingResponse
	ParserStatusParsingResponse
	ParserStatusComplete
	ParserStatusError
)

// String returns a human-readable status description
func (s ParserProgressStatus) String() string {
	switch s {
	case ParserStatusIdle:
		return "Ready"
	case ParserStatusConnecting:
		return "Connecting"
	case ParserStatusSendingRequest:
		return "Sending request"
	case ParserStatusWaitingResponse:
		return "Waiting for response"
	case ParserStatusParsingResponse:
		return "Parsing response"
	case ParserStatusComplete:
		return "Complete"
	case ParserStatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ParserProgressUpdate contains information about parsing progress
type ParserProgressUpdate struct {
	// Status is the current processing status
	Status ParserProgressStatus

	// Provider is the AI provider being used
	Provider string

	// Model is the model name being used
	Model string

	// Message is a human-readable status message
	Message string

	// Detail provides additional context
	Detail string

	// Error contains any error that occurred
	Error error

	// === Transparency fields for AI request/response logging ===

	// RequestInfo contains details about the AI request being made
	RequestInfo *ParserRequestInfo

	// ResponseInfo contains details about the AI response received
	ResponseInfo *ParserResponseInfo
}

// ParserRequestInfo contains transparency details about a parser AI request
type ParserRequestInfo struct {
	// Endpoint URL (sanitized, no API keys)
	Endpoint string

	// Method HTTP method (POST, GET, etc.)
	Method string

	// SystemPromptPreview shows first N characters of the system prompt
	SystemPromptPreview string

	// UserInput the user's natural language input
	UserInput string

	// Parameters like temperature, max_tokens, etc.
	Parameters map[string]string
}

// ParserResponseInfo contains transparency details about a parser AI response
type ParserResponseInfo struct {
	// StatusCode HTTP status code
	StatusCode int

	// StatusText HTTP status text
	StatusText string

	// Latency time taken for the request
	Latency time.Duration

	// RawResponse the raw response content (for transparency)
	RawResponse string

	// ParsedResult the parsed clip request result
	ParsedResult *ClipRequest

	// ErrorMessage if any
	ErrorMessage string
}

// ParserProgressCallback is called with progress updates during parsing
type ParserProgressCallback func(update ParserProgressUpdate)

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

	// Check for Azure Anthropic (Claude via Azure)
	azureAnthropicEndpoint := os.Getenv("AZURE_ANTHROPIC_ENDPOINT")
	azureAnthropicAPIKey := os.Getenv("AZURE_ANTHROPIC_API_KEY")
	azureAnthropicModel := os.Getenv("AZURE_ANTHROPIC_MODEL")

	if azureAnthropicEndpoint != "" {
		if azureAnthropicAPIKey == "" {
			return nil, fmt.Errorf("AZURE_ANTHROPIC_API_KEY environment variable not set")
		}
		if azureAnthropicModel == "" {
			azureAnthropicModel = "claude-sonnet-4-20250514" // Default to Claude Sonnet 4
		}

		azureAnthropicEndpoint = strings.TrimSuffix(azureAnthropicEndpoint, "/")

		if debug {
			fmt.Println("\n[DEBUG] Azure Anthropic Configuration:")
			fmt.Printf("  AZURE_ANTHROPIC_ENDPOINT: %s\n", azureAnthropicEndpoint)
			fmt.Printf("  AZURE_ANTHROPIC_API_KEY:  %s...%s\n", azureAnthropicAPIKey[:4], azureAnthropicAPIKey[len(azureAnthropicAPIKey)-4:])
			fmt.Printf("  AZURE_ANTHROPIC_MODEL:    %s\n", azureAnthropicModel)
			fmt.Println()
		}

		// Create Anthropic client with Azure endpoint
		anthropicClient := anthropic.NewClient(
			option.WithAPIKey(azureAnthropicAPIKey),
			option.WithBaseURL(azureAnthropicEndpoint),
		)

		return &Parser{
			provider:        ProviderAzureAnthropic,
			endpoint:        azureAnthropicEndpoint,
			apiKey:          azureAnthropicAPIKey,
			model:           azureAnthropicModel,
			anthropicClient: &anthropicClient,
			client:          &http.Client{Timeout: 60 * time.Second},
		}, nil
	}

	// Fall back to Azure OpenAI
	endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
	model := os.Getenv("AZURE_OPENAI_MODEL")
	apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")

	if endpoint == "" {
		return nil, fmt.Errorf("no AI backend configured. Set LLM_ENDPOINT for local LLM, AZURE_ANTHROPIC_ENDPOINT for Azure Anthropic, or AZURE_OPENAI_ENDPOINT for Azure OpenAI")
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

// GetProvider returns the current provider
func (p *Parser) GetProvider() Provider {
	return p.provider
}

// GetModel returns the current model name
func (p *Parser) GetModel() string {
	return p.model
}

// GetProviderDisplayName returns a user-friendly provider name
func (p *Parser) GetProviderDisplayName() string {
	switch p.provider {
	case ProviderLocal:
		return "Local LLM"
	case ProviderAzure:
		return "Azure OpenAI"
	case ProviderAzureAnthropic:
		return "Azure Anthropic"
	default:
		return string(p.provider)
	}
}

// ParseClipRequest parses a natural language clip request into timestamps
func (p *Parser) ParseClipRequest(ctx context.Context, userInput string, videoDuration time.Duration) (*ClipRequest, error) {
	return p.ParseClipRequestWithProgress(ctx, userInput, videoDuration, nil)
}

// ParseClipRequestWithProgress parses a clip request with progress callbacks
func (p *Parser) ParseClipRequestWithProgress(ctx context.Context, userInput string, videoDuration time.Duration, onProgress ParserProgressCallback) (*ClipRequest, error) {
	// Send initial progress
	p.sendProgress(onProgress, ParserProgressUpdate{
		Status:   ParserStatusConnecting,
		Provider: string(p.provider),
		Model:    p.model,
		Message:  "Connecting to AI",
		Detail:   "Preparing request...",
	})

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

	// Build endpoint URL for transparency (sanitized)
	var endpoint string
	switch p.provider {
	case ProviderLocal:
		endpoint = p.endpoint + "/v1/chat/completions"
	case ProviderAzure:
		endpoint = p.endpoint + "/openai/responses"
	case ProviderAzureAnthropic:
		endpoint = p.endpoint + "/v1/messages"
	}

	// Send progress: sending request with transparency details
	p.sendProgress(onProgress, ParserProgressUpdate{
		Status:   ParserStatusSendingRequest,
		Provider: string(p.provider),
		Model:    p.model,
		Message:  "Sending request to " + p.GetProviderDisplayName(),
		Detail:   "Processing: " + userInput,
		RequestInfo: &ParserRequestInfo{
			Endpoint:            endpoint,
			Method:              "POST",
			SystemPromptPreview: truncatePrompt(systemPrompt, 100),
			UserInput:           userInput,
			Parameters: map[string]string{
				"model":       p.model,
				"max_tokens":  "512",
				"temperature": "0.1",
			},
		},
	})

	var result *ClipRequest
	var err error
	var rawResponse string
	var statusCode int
	var statusText string
	startTime := time.Now()

	switch p.provider {
	case ProviderLocal:
		p.sendProgress(onProgress, ParserProgressUpdate{
			Status:   ParserStatusWaitingResponse,
			Provider: string(p.provider),
			Model:    p.model,
			Message:  "Waiting for Local LLM response",
			Detail:   "Model: " + p.model,
		})
		result, rawResponse, statusCode, statusText, err = p.parseWithOpenAITransparent(ctx, systemPrompt, userInput)
	case ProviderAzure:
		p.sendProgress(onProgress, ParserProgressUpdate{
			Status:   ParserStatusWaitingResponse,
			Provider: string(p.provider),
			Model:    p.model,
			Message:  "Waiting for Azure OpenAI response",
			Detail:   "Model: " + p.model,
		})
		result, rawResponse, statusCode, statusText, err = p.parseWithAzureTransparent(ctx, systemPrompt, userInput)
	case ProviderAzureAnthropic:
		p.sendProgress(onProgress, ParserProgressUpdate{
			Status:   ParserStatusWaitingResponse,
			Provider: string(p.provider),
			Model:    p.model,
			Message:  "Waiting for Claude response",
			Detail:   "Model: " + p.model,
		})
		result, rawResponse, statusCode, statusText, err = p.parseWithAzureAnthropicTransparent(ctx, systemPrompt, userInput)
	default:
		return nil, fmt.Errorf("unknown provider: %s", p.provider)
	}

	latency := time.Since(startTime)

	if err != nil {
		p.sendProgress(onProgress, ParserProgressUpdate{
			Status:   ParserStatusError,
			Provider: string(p.provider),
			Model:    p.model,
			Message:  "Request failed",
			Detail:   err.Error(),
			Error:    err,
			ResponseInfo: &ParserResponseInfo{
				StatusCode:   statusCode,
				StatusText:   statusText,
				Latency:      latency,
				RawResponse:  rawResponse,
				ErrorMessage: err.Error(),
			},
		})
		return nil, err
	}

	// Send response received progress with transparency details
	p.sendProgress(onProgress, ParserProgressUpdate{
		Status:   ParserStatusParsingResponse,
		Provider: string(p.provider),
		Model:    p.model,
		Message:  "Response received from " + p.GetProviderDisplayName(),
		Detail:   fmt.Sprintf("Latency: %.2fs", latency.Seconds()),
		ResponseInfo: &ParserResponseInfo{
			StatusCode:   statusCode,
			StatusText:   statusText,
			Latency:      latency,
			RawResponse:  truncatePrompt(rawResponse, 200),
			ParsedResult: result,
		},
	})

	// Send completion progress
	p.sendProgress(onProgress, ParserProgressUpdate{
		Status:   ParserStatusComplete,
		Provider: string(p.provider),
		Model:    p.model,
		Message:  "Parsing complete",
		Detail:   fmt.Sprintf("Start: %s, End: %s", result.StartTime, result.EndTime),
	})

	return result, nil
}

// sendProgress sends a progress update if callback is configured
func (p *Parser) sendProgress(onProgress ParserProgressCallback, update ParserProgressUpdate) {
	if onProgress == nil {
		return
	}
	onProgress(update)
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

// parseWithAzureAnthropic handles Azure Anthropic (Claude) API
func (p *Parser) parseWithAzureAnthropic(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	if p.anthropicClient == nil {
		return nil, fmt.Errorf("Anthropic client not initialized")
	}

	if debug {
		fmt.Printf("[DEBUG] Azure Anthropic request to model: %s\n", p.model)
	}

	// Create the message request
	message, err := p.anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("Azure Anthropic request failed: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Azure Anthropic response: %+v\n", message)
	}

	// Extract text content from response
	var content string
	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			content = b.Text
			break
		}
	}

	if content == "" {
		return nil, fmt.Errorf("no content in Azure Anthropic response")
	}

	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
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

// truncatePrompt truncates a prompt string to maxLen characters
func truncatePrompt(s string, maxLen int) string {
	// Remove newlines for preview
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
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

Option 2: Azure Anthropic (Claude)
  export AZURE_ANTHROPIC_ENDPOINT="https://your-resource.services.ai.azure.com"
  export AZURE_ANTHROPIC_API_KEY="your-api-key"
  export AZURE_ANTHROPIC_MODEL="claude-sonnet-4-20250514"  # Optional, defaults to claude-sonnet-4

Option 3: Azure OpenAI
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

	// Check Azure Anthropic config
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" {
		if os.Getenv("AZURE_ANTHROPIC_API_KEY") == "" {
			return fmt.Errorf("AZURE_ANTHROPIC_API_KEY not set")
		}
		return nil // Azure Anthropic configured
	}

	// Check Azure OpenAI config
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

// GetAvailableProviders returns a list of configured providers
func GetAvailableProviders() []Provider {
	var providers []Provider

	// Check for local LLM
	if os.Getenv("LLM_ENDPOINT") != "" {
		providers = append(providers, ProviderLocal)
	}

	// Check for Azure Anthropic
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" && os.Getenv("AZURE_ANTHROPIC_API_KEY") != "" {
		providers = append(providers, ProviderAzureAnthropic)
	}

	// Check for Azure OpenAI
	if os.Getenv("AZURE_OPENAI_ENDPOINT") != "" && os.Getenv("AZURE_OPENAI_API_KEY") != "" && os.Getenv("AZURE_OPENAI_MODEL") != "" {
		providers = append(providers, ProviderAzure)
	}

	return providers
}

// GetProviderDisplayNameStatic returns a user-friendly provider name for a given provider
func GetProviderDisplayNameStatic(p Provider) string {
	switch p {
	case ProviderLocal:
		endpoint := os.Getenv("LLM_ENDPOINT")
		if endpoint != "" {
			return fmt.Sprintf("Local LLM (%s)", endpoint)
		}
		return "Local LLM"
	case ProviderAzure:
		return "Azure OpenAI"
	case ProviderAzureAnthropic:
		return "Azure Anthropic (Claude)"
	default:
		return string(p)
	}
}

// NewParserWithProvider creates a parser for a specific provider
func NewParserWithProvider(provider Provider) (*Parser, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	switch provider {
	case ProviderLocal:
		localEndpoint := os.Getenv("LLM_ENDPOINT")
		localModel := os.Getenv("LLM_MODEL")

		if localEndpoint == "" {
			return nil, fmt.Errorf("LLM_ENDPOINT environment variable not set for local LLM")
		}

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
			client:   &http.Client{Timeout: 120 * time.Second},
		}, nil

	case ProviderAzureAnthropic:
		azureAnthropicEndpoint := os.Getenv("AZURE_ANTHROPIC_ENDPOINT")
		azureAnthropicAPIKey := os.Getenv("AZURE_ANTHROPIC_API_KEY")
		azureAnthropicModel := os.Getenv("AZURE_ANTHROPIC_MODEL")

		if azureAnthropicEndpoint == "" {
			return nil, fmt.Errorf("AZURE_ANTHROPIC_ENDPOINT environment variable not set")
		}
		if azureAnthropicAPIKey == "" {
			return nil, fmt.Errorf("AZURE_ANTHROPIC_API_KEY environment variable not set")
		}
		if azureAnthropicModel == "" {
			azureAnthropicModel = "claude-sonnet-4-20250514"
		}

		azureAnthropicEndpoint = strings.TrimSuffix(azureAnthropicEndpoint, "/")

		if debug {
			fmt.Println("\n[DEBUG] Azure Anthropic Configuration:")
			fmt.Printf("  AZURE_ANTHROPIC_ENDPOINT: %s\n", azureAnthropicEndpoint)
			fmt.Printf("  AZURE_ANTHROPIC_API_KEY:  %s...%s\n", azureAnthropicAPIKey[:4], azureAnthropicAPIKey[len(azureAnthropicAPIKey)-4:])
			fmt.Printf("  AZURE_ANTHROPIC_MODEL:    %s\n", azureAnthropicModel)
			fmt.Println()
		}

		anthropicClient := anthropic.NewClient(
			option.WithAPIKey(azureAnthropicAPIKey),
			option.WithBaseURL(azureAnthropicEndpoint),
		)

		return &Parser{
			provider:        ProviderAzureAnthropic,
			endpoint:        azureAnthropicEndpoint,
			apiKey:          azureAnthropicAPIKey,
			model:           azureAnthropicModel,
			anthropicClient: &anthropicClient,
			client:          &http.Client{Timeout: 60 * time.Second},
		}, nil

	case ProviderAzure:
		endpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
		apiKey := os.Getenv("AZURE_OPENAI_API_KEY")
		model := os.Getenv("AZURE_OPENAI_MODEL")
		apiVersion := os.Getenv("AZURE_OPENAI_API_VERSION")

		if endpoint == "" {
			return nil, fmt.Errorf("AZURE_OPENAI_ENDPOINT environment variable not set")
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

	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

// parseWithOpenAITransparent handles OpenAI-compatible APIs with transparency info
func (p *Parser) parseWithOpenAITransparent(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, string, int, string, error) {
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
		return nil, "", 0, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1/chat/completions", p.endpoint)

	if debug {
		fmt.Printf("[DEBUG] Request URL: %s\n", apiURL)
		fmt.Printf("[DEBUG] Request body: %s\n\n", string(jsonBody))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("AI request failed (is the LLM server running?): %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, resp.Status, fmt.Errorf("failed to read response: %w", err)
	}

	rawResponse := string(body)

	if debug {
		fmt.Printf("[DEBUG] Response status: %s\n", resp.Status)
		fmt.Printf("[DEBUG] Response body: %s\n\n", rawResponse)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("AI request failed: %s\n  URL: %s\n  Response: %s", resp.Status, apiURL, rawResponse)
	}

	var apiResp openAIResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("failed to parse API response: %w\nResponse was: %s", err, rawResponse)
	}

	if apiResp.Error != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	if len(apiResp.Choices) == 0 {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("no choices in AI response")
	}

	content := cleanJSONResponse(apiResp.Choices[0].Message.Content)

	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
	}

	var clipReq ClipRequest
	if err := json.Unmarshal([]byte(content), &clipReq); err != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("failed to parse AI response: %w\nResponse was: %s", err, content)
	}

	if clipReq.Error != "" {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("AI could not parse request: %s", clipReq.Error)
	}

	return &clipReq, rawResponse, resp.StatusCode, resp.Status, nil
}

// parseWithAzureTransparent handles Azure OpenAI Responses API with transparency info
func (p *Parser) parseWithAzureTransparent(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, string, int, string, error) {
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
		return nil, "", 0, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/openai/responses?api-version=%s", p.endpoint, p.apiVersion)

	if debug {
		fmt.Printf("[DEBUG] Request URL: %s\n", apiURL)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", resp.StatusCode, resp.Status, fmt.Errorf("failed to read response: %w", err)
	}

	rawResponse := string(body)

	if resp.StatusCode != http.StatusOK {
		if debug {
			fmt.Printf("\n[DEBUG] Request failed:\n")
			fmt.Printf("  URL:      %s\n", apiURL)
			fmt.Printf("  Status:   %s\n", resp.Status)
			fmt.Printf("  Response: %s\n\n", rawResponse)
		}
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("AI request failed: %s\n  URL: %s\n  Response: %s", resp.Status, apiURL, rawResponse)
	}

	if debug {
		fmt.Printf("\n[DEBUG] Raw API Response:\n%s\n\n", rawResponse)
	}

	var apiResp azureResponse
	if err := json.Unmarshal(body, &apiResp); err != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("failed to parse API response: %w\nResponse was: %s", err, rawResponse)
	}

	if apiResp.Error != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("API error: %s - %s", apiResp.Error.Code, apiResp.Error.Message)
	}

	content := extractAzureContent(apiResp)
	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
	}
	if content == "" {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("no content in AI response\nFull response: %s", rawResponse)
	}

	content = cleanJSONResponse(content)

	var clipReq ClipRequest
	if err := json.Unmarshal([]byte(content), &clipReq); err != nil {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("failed to parse AI response: %w\nResponse was: %s", err, content)
	}

	if clipReq.Error != "" {
		return nil, rawResponse, resp.StatusCode, resp.Status, fmt.Errorf("AI could not parse request: %s", clipReq.Error)
	}

	return &clipReq, rawResponse, resp.StatusCode, resp.Status, nil
}

// parseWithAzureAnthropicTransparent handles Azure Anthropic (Claude) API with transparency info
func (p *Parser) parseWithAzureAnthropicTransparent(ctx context.Context, systemPrompt, userInput string) (*ClipRequest, string, int, string, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	if p.anthropicClient == nil {
		return nil, "", 0, "", fmt.Errorf("Anthropic client not initialized")
	}

	if debug {
		fmt.Printf("[DEBUG] Azure Anthropic request to model: %s\n", p.model)
	}

	// Create the message request
	message, err := p.anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(p.model),
		MaxTokens: 1024,
		System: []anthropic.TextBlockParam{
			{Text: systemPrompt},
		},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userInput)),
		},
	})
	if err != nil {
		return nil, "", 0, "", fmt.Errorf("Azure Anthropic request failed: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Azure Anthropic response: %+v\n", message)
	}

	// Extract text content from response
	var content string
	var rawResponse string
	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			content = b.Text
			rawResponse = b.Text
		}
	}

	if content == "" {
		return nil, rawResponse, 200, "OK", fmt.Errorf("no content in Azure Anthropic response")
	}

	if debug {
		fmt.Printf("[DEBUG] Extracted content: %q\n\n", content)
	}

	content = cleanJSONResponse(content)

	var clipReq ClipRequest
	if err := json.Unmarshal([]byte(content), &clipReq); err != nil {
		return nil, rawResponse, 200, "OK", fmt.Errorf("failed to parse AI response: %w\nResponse was: %s", err, content)
	}

	if clipReq.Error != "" {
		return nil, rawResponse, 200, "OK", fmt.Errorf("AI could not parse request: %s", clipReq.Error)
	}

	return &clipReq, rawResponse, 200, "OK", nil
}
