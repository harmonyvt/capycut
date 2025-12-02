//go:build integration
// +build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"capycut/ai"
	"capycut/video"
)

// TestIntegration_VideoClipping tests the full video clipping workflow
func TestIntegration_VideoClipping(t *testing.T) {
	// Skip if no test video is available
	testVideo := os.Getenv("TEST_VIDEO_PATH")
	if testVideo == "" {
		t.Skip("TEST_VIDEO_PATH not set, skipping integration test")
	}

	if _, err := os.Stat(testVideo); os.IsNotExist(err) {
		t.Skipf("Test video not found at %s", testVideo)
	}

	// Verify FFmpeg is available
	if err := video.CheckFFmpeg(); err != nil {
		t.Skipf("FFmpeg not available: %v", err)
	}

	// Get video info
	info, err := video.GetVideoInfo(testVideo)
	if err != nil {
		t.Fatalf("Failed to get video info: %v", err)
	}

	t.Logf("Test video: %s, Duration: %v", info.Filename, info.Duration)

	// Test clipping
	outputPath := filepath.Join(t.TempDir(), "test_clip.mp4")
	params := video.ClipParams{
		InputPath:  testVideo,
		StartTime:  "00:00:00",
		EndTime:    "00:00:05",
		OutputPath: outputPath,
	}

	err = video.ClipVideo(params)
	if err != nil {
		t.Fatalf("Failed to clip video: %v", err)
	}

	// Verify output exists
	if _, err := os.Stat(outputPath); os.IsNotExist(err) {
		t.Fatal("Output clip was not created")
	}

	// Verify output duration
	clipInfo, err := video.GetVideoInfo(outputPath)
	if err != nil {
		t.Fatalf("Failed to get clip info: %v", err)
	}

	expectedDuration := 5 * time.Second
	tolerance := 1 * time.Second
	if clipInfo.Duration < expectedDuration-tolerance || clipInfo.Duration > expectedDuration+tolerance {
		t.Errorf("Clip duration %v is not within expected range of %v ± %v", clipInfo.Duration, expectedDuration, tolerance)
	}

	t.Logf("Successfully created clip: %s, Duration: %v", clipInfo.Filename, clipInfo.Duration)
}

// TestIntegration_MockLLMServer tests the AI parser with a mock LLM server
func TestIntegration_MockLLMServer(t *testing.T) {
	// Create mock LLM server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
			http.Error(w, "Not found", http.StatusNotFound)
			return
		}

		// Return a valid clip response
		response := map[string]interface{}{
			"id":     "mock-test-123",
			"object": "chat.completion",
			"choices": []map[string]interface{}{
				{
					"index": 0,
					"message": map[string]string{
						"role":    "assistant",
						"content": `{"start_time": "00:01:00", "end_time": "00:02:30"}`,
					},
					"finish_reason": "stop",
				},
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}))
	defer mockServer.Close()

	// Set up environment for mock server
	os.Setenv("LLM_PROVIDER", "local")
	os.Setenv("LLM_ENDPOINT", mockServer.URL)
	os.Setenv("LLM_MODEL", "test-model")
	defer func() {
		os.Unsetenv("LLM_PROVIDER")
		os.Unsetenv("LLM_ENDPOINT")
		os.Unsetenv("LLM_MODEL")
	}()

	// Create parser and test
	parser, err := ai.NewParser()
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	clipReq, err := parser.ParseClipRequest(ctx, "from 1 minute to 2 minutes 30 seconds", 10*time.Minute)
	if err != nil {
		t.Fatalf("Failed to parse clip request: %v", err)
	}

	if clipReq.StartTime != "00:01:00" {
		t.Errorf("Expected start time 00:01:00, got %s", clipReq.StartTime)
	}
	if clipReq.EndTime != "00:02:30" {
		t.Errorf("Expected end time 00:02:30, got %s", clipReq.EndTime)
	}

	t.Logf("Successfully parsed clip request: %s to %s", clipReq.StartTime, clipReq.EndTime)
}

// TestIntegration_CLIHelp tests that the CLI help is working
func TestIntegration_CLIHelp(t *testing.T) {
	// Build the binary first
	binaryName := "capycut"
	if runtime.GOOS == "windows" {
		binaryName = "capycut.exe"
	}

	// Check if binary exists in current directory
	if _, err := os.Stat(binaryName); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping CLI integration test. Run 'go build' first.")
	}

	// Test --help flag
	cmd := exec.Command("./"+binaryName, "--help")
	output, err := cmd.CombinedOutput()
	if err != nil {
		// --help might return non-zero exit code on some systems
		t.Logf("Help command output: %s", string(output))
	}

	outputStr := string(output)

	// Verify help contains expected content
	expectedStrings := []string{
		"capycut",
		"--file",
		"--prompt",
		"--provider",
		"--setup",
		"LLM_PROVIDER",
		"LLM_ENDPOINT",
		"AZURE_OPENAI",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(outputStr, expected) {
			t.Errorf("Help output missing expected string: %s", expected)
		}
	}

	t.Log("CLI help test passed")
}

// TestIntegration_CLIVersion tests the version command
func TestIntegration_CLIVersion(t *testing.T) {
	binaryName := "capycut"
	if runtime.GOOS == "windows" {
		binaryName = "capycut.exe"
	}

	if _, err := os.Stat(binaryName); os.IsNotExist(err) {
		t.Skip("Binary not found, skipping CLI integration test")
	}

	cmd := exec.Command("./"+binaryName, "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v\nOutput: %s", err, string(output))
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "capycut") {
		t.Error("Version output should contain 'capycut'")
	}

	t.Logf("Version output: %s", outputStr)
}

// TestIntegration_ShellConfigGeneration tests config generation for all shells
func TestIntegration_ShellConfigGeneration(t *testing.T) {
	testCases := []struct {
		name        string
		shellType   ShellType
		provider    string
		endpoint    string
		apiKey      string
		model       string
		apiVersion  string
		expected    []string
		notExpected []string
	}{
		{
			name:        "Bash/Zsh Local",
			shellType:   ShellBashZsh,
			provider:    "local",
			endpoint:    "http://localhost:1234",
			model:       "llama3",
			expected:    []string{"export LLM_PROVIDER=local", "export LLM_ENDPOINT=http://localhost:1234", "export LLM_MODEL=llama3"},
			notExpected: []string{"$env:", "set -gx"},
		},
		{
			name:        "PowerShell Azure",
			shellType:   ShellPowerShell,
			provider:    "azure",
			endpoint:    "https://test.openai.azure.com",
			apiKey:      "test-key",
			model:       "gpt-4o",
			apiVersion:  "2025-04-01-preview",
			expected:    []string{`$env:LLM_PROVIDER = "azure"`, `$env:AZURE_OPENAI_ENDPOINT`, `$env:AZURE_OPENAI_API_KEY`},
			notExpected: []string{"export ", "set -gx"},
		},
		{
			name:        "Fish Local",
			shellType:   ShellFish,
			provider:    "local",
			endpoint:    "http://localhost:11434",
			model:       "mistral",
			expected:    []string{"set -gx LLM_PROVIDER local", "set -gx LLM_ENDPOINT http://localhost:11434"},
			notExpected: []string{"export ", "$env:"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := generateEnvExports(tc.provider, tc.endpoint, tc.apiKey, tc.model, tc.apiVersion, tc.shellType)

			for _, exp := range tc.expected {
				if !strings.Contains(result, exp) {
					t.Errorf("Expected %q in output, got:\n%s", exp, result)
				}
			}

			for _, notExp := range tc.notExpected {
				if strings.Contains(result, notExp) {
					t.Errorf("Did not expect %q in output, got:\n%s", notExp, result)
				}
			}
		})
	}
}

// TestIntegration_OutputPathGeneration tests output path generation
func TestIntegration_OutputPathGeneration(t *testing.T) {
	testCases := []struct {
		input     string
		startTime string
		endTime   string
	}{
		{"video.mp4", "00:01:00", "00:02:30"},
		{"my video.mp4", "00:00:00", "00:05:00"},
		{"/path/to/video.mkv", "01:00:00", "01:30:00"},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			output := video.GenerateOutputPath(tc.input, tc.startTime, tc.endTime)

			// Should have same extension
			inputExt := filepath.Ext(tc.input)
			outputExt := filepath.Ext(output)
			if inputExt != outputExt {
				t.Errorf("Expected extension %s, got %s", inputExt, outputExt)
			}

			// Should contain clip indicator
			if !strings.Contains(output, "clip") && !strings.Contains(output, "00") {
				t.Errorf("Output path should indicate it's a clip: %s", output)
			}

			t.Logf("Input: %s -> Output: %s", tc.input, output)
		})
	}
}

// TestIntegration_TimestampParsing tests timestamp parsing with various formats
func TestIntegration_TimestampParsing(t *testing.T) {
	testCases := []struct {
		input    string
		expected time.Duration
		hasError bool
	}{
		{"00:00:00", 0, false},
		{"00:01:30", 90 * time.Second, false},
		{"01:30:00", 90 * time.Minute, false},
		{"1:30", 90 * time.Second, false},
		{"90", 90 * time.Second, false},
		{"invalid", 0, true},
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			duration, err := video.ParseTimestamp(tc.input)

			if tc.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if duration != tc.expected {
					t.Errorf("Expected %v, got %v", tc.expected, duration)
				}
			}
		})
	}
}

// TestIntegration_EnvironmentDetection tests environment variable detection
func TestIntegration_EnvironmentDetection(t *testing.T) {
	// Save original env
	origProvider := os.Getenv("LLM_PROVIDER")
	origEndpoint := os.Getenv("LLM_ENDPOINT")
	origModel := os.Getenv("LLM_MODEL")
	defer func() {
		os.Setenv("LLM_PROVIDER", origProvider)
		os.Setenv("LLM_ENDPOINT", origEndpoint)
		os.Setenv("LLM_MODEL", origModel)
	}()

	// Test auto-detection when LLM_ENDPOINT is set
	os.Setenv("LLM_PROVIDER", "")
	os.Setenv("LLM_ENDPOINT", "http://localhost:1234")
	os.Setenv("LLM_MODEL", "test-model")

	err := ai.CheckConfig()
	if err != nil {
		t.Errorf("Expected config to be valid with LLM_ENDPOINT set: %v", err)
	}

	// Test Azure detection when no LLM_ENDPOINT
	os.Unsetenv("LLM_ENDPOINT")
	os.Unsetenv("LLM_MODEL")
	os.Setenv("AZURE_OPENAI_ENDPOINT", "https://test.openai.azure.com")
	os.Setenv("AZURE_OPENAI_API_KEY", "test-key")
	os.Setenv("AZURE_OPENAI_MODEL", "gpt-4o")
	defer func() {
		os.Unsetenv("AZURE_OPENAI_ENDPOINT")
		os.Unsetenv("AZURE_OPENAI_API_KEY")
		os.Unsetenv("AZURE_OPENAI_MODEL")
	}()

	err = ai.CheckConfig()
	if err != nil {
		t.Errorf("Expected config to be valid with Azure env vars: %v", err)
	}
}

// BenchmarkVideoClipping benchmarks the video clipping performance
func BenchmarkVideoClipping(b *testing.B) {
	testVideo := os.Getenv("TEST_VIDEO_PATH")
	if testVideo == "" {
		b.Skip("TEST_VIDEO_PATH not set")
	}

	if err := video.CheckFFmpeg(); err != nil {
		b.Skip("FFmpeg not available")
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		outputPath := filepath.Join(b.TempDir(), fmt.Sprintf("bench_clip_%d.mp4", i))
		params := video.ClipParams{
			InputPath:  testVideo,
			StartTime:  "00:00:00",
			EndTime:    "00:00:02",
			OutputPath: outputPath,
		}
		video.ClipVideo(params)
	}
}
