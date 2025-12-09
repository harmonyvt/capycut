package ai

import (
	"os"
	"testing"
	"time"
)

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			input:    0,
			expected: "00:00:00",
		},
		{
			name:     "seconds only",
			input:    45 * time.Second,
			expected: "00:00:45",
		},
		{
			name:     "minutes and seconds",
			input:    3*time.Minute + 30*time.Second,
			expected: "00:03:30",
		},
		{
			name:     "hours minutes seconds",
			input:    1*time.Hour + 30*time.Minute + 45*time.Second,
			expected: "01:30:45",
		},
		{
			name:     "large duration",
			input:    10*time.Hour + 5*time.Minute + 3*time.Second,
			expected: "10:05:03",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCleanJSONResponse(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "plain JSON",
			input:    `{"start_time": "00:01:00", "end_time": "00:02:00"}`,
			expected: `{"start_time": "00:01:00", "end_time": "00:02:00"}`,
		},
		{
			name:     "with markdown code block",
			input:    "```json\n{\"start_time\": \"00:01:00\", \"end_time\": \"00:02:00\"}\n```",
			expected: `{"start_time": "00:01:00", "end_time": "00:02:00"}`,
		},
		{
			name:     "with plain code block",
			input:    "```\n{\"start_time\": \"00:01:00\"}\n```",
			expected: `{"start_time": "00:01:00"}`,
		},
		{
			name:     "with whitespace",
			input:    "  {\"start_time\": \"00:00:00\"}  ",
			expected: `{"start_time": "00:00:00"}`,
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanJSONResponse(tt.input)
			if result != tt.expected {
				t.Errorf("cleanJSONResponse(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestGetAPIKeyHelp(t *testing.T) {
	help := GetAPIKeyHelp()

	// Check that help contains expected Azure OpenAI content
	expectedSubstrings := []string{
		"AZURE_OPENAI_ENDPOINT",
		"AZURE_OPENAI_API_KEY",
		"AZURE_OPENAI_MODEL",
		"Azure",
	}

	for _, substr := range expectedSubstrings {
		if !containsSubstring(help, substr) {
			t.Errorf("GetAPIKeyHelp() should contain %q", substr)
		}
	}
}

func TestCheckConfig(t *testing.T) {
	// Save original env vars
	origEndpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	origKey := os.Getenv("AZURE_OPENAI_API_KEY")
	origModel := os.Getenv("AZURE_OPENAI_MODEL")

	// Clean up after test
	defer func() {
		os.Setenv("AZURE_OPENAI_ENDPOINT", origEndpoint)
		os.Setenv("AZURE_OPENAI_API_KEY", origKey)
		os.Setenv("AZURE_OPENAI_MODEL", origModel)
	}()

	// Ensure local LLM is not configured so we test Azure path
	os.Unsetenv("LLM_ENDPOINT")
	os.Unsetenv("LLM_MODEL")

	// Test missing endpoint
	os.Setenv("AZURE_OPENAI_ENDPOINT", "")
	os.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	os.Setenv("AZURE_OPENAI_MODEL", "gpt-5-codex")
	if err := CheckConfig(); err == nil {
		t.Error("CheckConfig() should error when AZURE_OPENAI_ENDPOINT is missing")
	}

	// Test missing API key
	os.Setenv("AZURE_OPENAI_ENDPOINT", "https://test.openai.azure.com")
	os.Setenv("AZURE_OPENAI_API_KEY", "")
	os.Setenv("AZURE_OPENAI_MODEL", "gpt-5-codex")
	if err := CheckConfig(); err == nil {
		t.Error("CheckConfig() should error when AZURE_OPENAI_API_KEY is missing")
	}

	// Test missing model
	os.Setenv("AZURE_OPENAI_ENDPOINT", "https://test.openai.azure.com")
	os.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	os.Setenv("AZURE_OPENAI_MODEL", "")
	if err := CheckConfig(); err == nil {
		t.Error("CheckConfig() should error when AZURE_OPENAI_MODEL is missing")
	}

	// Test all present
	os.Setenv("AZURE_OPENAI_ENDPOINT", "https://test.openai.azure.com")
	os.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	os.Setenv("AZURE_OPENAI_MODEL", "gpt-5-codex")
	if err := CheckConfig(); err != nil {
		t.Errorf("CheckConfig() should not error when all vars are set: %v", err)
	}
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestClipRequestStruct(t *testing.T) {
	// Test that ClipRequest can be properly initialized
	req := ClipRequest{
		StartTime: "00:01:00",
		EndTime:   "00:05:00",
		Error:     "",
	}

	if req.StartTime != "00:01:00" {
		t.Errorf("StartTime = %q, want %q", req.StartTime, "00:01:00")
	}
	if req.EndTime != "00:05:00" {
		t.Errorf("EndTime = %q, want %q", req.EndTime, "00:05:00")
	}
	if req.Error != "" {
		t.Errorf("Error = %q, want empty string", req.Error)
	}
}

func TestClipRequestWithError(t *testing.T) {
	req := ClipRequest{
		StartTime: "",
		EndTime:   "",
		Error:     "Could not parse the request",
	}

	if req.Error == "" {
		t.Error("Error should not be empty")
	}
	if req.StartTime != "" || req.EndTime != "" {
		t.Error("StartTime and EndTime should be empty when Error is set")
	}
}

func TestNewParser(t *testing.T) {
	// Save original env vars
	origEndpoint := os.Getenv("AZURE_OPENAI_ENDPOINT")
	origKey := os.Getenv("AZURE_OPENAI_API_KEY")
	origModel := os.Getenv("AZURE_OPENAI_MODEL")
	origLLMEndpoint := os.Getenv("LLM_ENDPOINT")
	origLLMModel := os.Getenv("LLM_MODEL")

	// Clean up after test
	defer func() {
		os.Setenv("AZURE_OPENAI_ENDPOINT", origEndpoint)
		os.Setenv("AZURE_OPENAI_API_KEY", origKey)
		os.Setenv("AZURE_OPENAI_MODEL", origModel)
		if origLLMEndpoint != "" {
			os.Setenv("LLM_ENDPOINT", origLLMEndpoint)
		} else {
			os.Unsetenv("LLM_ENDPOINT")
		}
		if origLLMModel != "" {
			os.Setenv("LLM_MODEL", origLLMModel)
		} else {
			os.Unsetenv("LLM_MODEL")
		}
	}()

	// Ensure local LLM is not configured so we test Azure path
	os.Unsetenv("LLM_ENDPOINT")
	os.Unsetenv("LLM_MODEL")

	// Test successful parser creation with Azure
	os.Setenv("AZURE_OPENAI_ENDPOINT", "https://test.openai.azure.com/")
	os.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	os.Setenv("AZURE_OPENAI_MODEL", "gpt-5-codex")

	parser, err := NewParser()
	if err != nil {
		t.Errorf("NewParser() unexpected error: %v", err)
	}
	if parser == nil {
		t.Error("NewParser() returned nil parser")
	}

	// Verify parser has client and model set
	if parser != nil && parser.model != "gpt-5-codex" {
		t.Errorf("NewParser() model = %q, want %q", parser.model, "gpt-5-codex")
	}
}
