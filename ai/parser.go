package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/azure"
)

// ClipRequest represents parsed clip parameters from natural language
type ClipRequest struct {
	StartTime string `json:"start_time"` // Format: HH:MM:SS
	EndTime   string `json:"end_time"`   // Format: HH:MM:SS
	Error     string `json:"error,omitempty"`
}

// Parser handles AI-powered natural language parsing using Azure OpenAI
type Parser struct {
	client *openai.Client
	model  string
}

// NewParser creates a new AI parser using Azure OpenAI SDK
func NewParser() (*Parser, error) {
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

	// Default API version if not specified
	if apiVersion == "" {
		apiVersion = "2024-08-01-preview"
	}

	// Ensure endpoint doesn't have trailing slash
	endpoint = strings.TrimSuffix(endpoint, "/")

	// Create client using Azure OpenAI SDK with API key
	client := openai.NewClient(
		azure.WithEndpoint(endpoint, apiVersion),
		azure.WithAPIKey(apiKey),
	)

	return &Parser{
		client: &client,
		model:  model,
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

	resp, err := p.client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model: openai.ChatModel(p.model),
		Messages: []openai.ChatCompletionMessageParamUnion{
			{
				OfSystem: &openai.ChatCompletionSystemMessageParam{
					Content: openai.ChatCompletionSystemMessageParamContentUnion{
						OfString: openai.String(systemPrompt),
					},
				},
			},
			{
				OfUser: &openai.ChatCompletionUserMessageParam{
					Content: openai.ChatCompletionUserMessageParamContentUnion{
						OfString: openai.String(userInput),
					},
				},
			},
		},
		MaxTokens:   openai.Int(256),
		Temperature: openai.Float(0.1), // Low temperature for consistent parsing
	})

	if err != nil {
		return nil, fmt.Errorf("AI request failed: %w", err)
	}

	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no response from AI")
	}

	content := resp.Choices[0].Message.Content
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

// cleanJSONResponse removes markdown code blocks if present
func cleanJSONResponse(s string) string {
	s = strings.TrimSpace(s)
	// Remove ```json and ``` markers if present
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

// GetAPIKeyHelp returns help text for setting up Azure OpenAI
func GetAPIKeyHelp() string {
	return `To use CapyCut, you need Azure OpenAI credentials.

Option 1: Create a .env file in the capycut directory:
  cp .env.example .env
  # Then edit .env and add your Azure OpenAI settings

Option 2: Set environment variables:
  export AZURE_OPENAI_ENDPOINT="https://your-resource.openai.azure.com"
  export AZURE_OPENAI_API_KEY="your-api-key"
  export AZURE_OPENAI_MODEL="gpt-4o"
  export AZURE_OPENAI_API_VERSION="2024-08-01-preview"  # optional

Get these from your Azure OpenAI resource in the Azure Portal.`
}

// CheckConfig validates that all required Azure OpenAI config is present
func CheckConfig() error {
	if os.Getenv("AZURE_OPENAI_ENDPOINT") == "" {
		return fmt.Errorf("AZURE_OPENAI_ENDPOINT not set")
	}
	if os.Getenv("AZURE_OPENAI_API_KEY") == "" {
		return fmt.Errorf("AZURE_OPENAI_API_KEY not set")
	}
	if os.Getenv("AZURE_OPENAI_MODEL") == "" {
		return fmt.Errorf("AZURE_OPENAI_MODEL not set")
	}
	return nil
}
