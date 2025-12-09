package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GeminiClient handles interactions with Google's Gemini API for video transcription
type GeminiClient struct {
	apiKey  string
	model   string
	client  *http.Client
	baseURL string
}

// Gemini API types for file upload
type geminiUploadInitResponse struct {
	File struct {
		Name        string `json:"name"`
		DisplayName string `json:"displayName"`
		MimeType    string `json:"mimeType"`
		SizeBytes   string `json:"sizeBytes"`
		CreateTime  string `json:"createTime"`
		UpdateTime  string `json:"updateTime"`
		State       string `json:"state"`
		URI         string `json:"uri"`
	} `json:"file"`
}

type geminiFileStatusResponse struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	MimeType    string `json:"mimeType"`
	SizeBytes   string `json:"sizeBytes"`
	CreateTime  string `json:"createTime"`
	UpdateTime  string `json:"updateTime"`
	State       string `json:"state"`
	URI         string `json:"uri"`
	Error       *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Gemini API types for content generation
type geminiContent struct {
	Parts []geminiPart `json:"parts"`
	Role  string       `json:"role,omitempty"`
}

type geminiPart struct {
	Text     string          `json:"text,omitempty"`
	FileData *geminiFileData `json:"fileData,omitempty"`
}

type geminiFileData struct {
	MimeType string `json:"mimeType"`
	FileURI  string `json:"fileUri"`
}

type geminiGenerateRequest struct {
	Contents         []geminiContent       `json:"contents"`
	GenerationConfig *geminiGenerateConfig `json:"generationConfig,omitempty"`
}

type geminiGenerateConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	TopP            float64 `json:"topP,omitempty"`
	TopK            int     `json:"topK,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type geminiGenerateResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason  string `json:"finishReason"`
		SafetyRatings []struct {
			Category    string `json:"category"`
			Probability string `json:"probability"`
		} `json:"safetyRatings"`
	} `json:"candidates"`
	UsageMetadata struct {
		PromptTokenCount     int `json:"promptTokenCount"`
		CandidatesTokenCount int `json:"candidatesTokenCount"`
		TotalTokenCount      int `json:"totalTokenCount"`
	} `json:"usageMetadata"`
	Error *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error,omitempty"`
}

// TranscriptResult contains the transcription output
type TranscriptResult struct {
	Text     string `json:"text"`
	Language string `json:"language,omitempty"`
}

// NewGeminiClient creates a new Gemini API client
func NewGeminiClient() (*GeminiClient, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY environment variable not set")
	}

	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = "gemini-2.0-flash" // Default to latest flash model
	}

	return &GeminiClient{
		apiKey:  apiKey,
		model:   model,
		client:  &http.Client{Timeout: 300 * time.Second}, // 5 min timeout for video uploads
		baseURL: "https://generativelanguage.googleapis.com",
	}, nil
}

// TranscribeVideo uploads a video to Gemini and generates a transcript
func (g *GeminiClient) TranscribeVideo(ctx context.Context, videoPath string, progressFn func(status string)) (*TranscriptResult, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	// Step 1: Upload the video file
	if progressFn != nil {
		progressFn("Uploading video to Gemini...")
	}

	fileURI, mimeType, err := g.uploadFile(ctx, videoPath)
	if err != nil {
		return nil, fmt.Errorf("failed to upload video: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Video uploaded successfully\n")
		fmt.Printf("[DEBUG] File URI: %s\n", fileURI)
		fmt.Printf("[DEBUG] MIME Type: %s\n", mimeType)
	}

	// Step 2: Wait for the file to be processed
	if progressFn != nil {
		progressFn("Processing video...")
	}

	if err := g.waitForFileProcessing(ctx, fileURI); err != nil {
		return nil, fmt.Errorf("failed to process video: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Video processing complete\n")
	}

	// Step 3: Generate transcript
	if progressFn != nil {
		progressFn("Generating transcript...")
	}

	transcript, err := g.generateTranscript(ctx, fileURI, mimeType)
	if err != nil {
		return nil, fmt.Errorf("failed to generate transcript: %w", err)
	}

	return transcript, nil
}

// uploadFile uploads a video file to Gemini's File API
func (g *GeminiClient) uploadFile(ctx context.Context, videoPath string) (fileURI string, mimeType string, err error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	// Open the file
	file, err := os.Open(videoPath)
	if err != nil {
		return "", "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Get file info
	fileInfo, err := file.Stat()
	if err != nil {
		return "", "", fmt.Errorf("failed to stat file: %w", err)
	}

	// Determine MIME type
	ext := strings.ToLower(filepath.Ext(videoPath))
	mimeType = mime.TypeByExtension(ext)
	if mimeType == "" {
		// Fallback for common video types
		switch ext {
		case ".mp4":
			mimeType = "video/mp4"
		case ".mov":
			mimeType = "video/quicktime"
		case ".avi":
			mimeType = "video/x-msvideo"
		case ".mkv":
			mimeType = "video/x-matroska"
		case ".webm":
			mimeType = "video/webm"
		case ".flv":
			mimeType = "video/x-flv"
		case ".wmv":
			mimeType = "video/x-ms-wmv"
		case ".m4v":
			mimeType = "video/x-m4v"
		default:
			mimeType = "video/mp4" // Default fallback
		}
	}

	displayName := filepath.Base(videoPath)

	if debug {
		fmt.Printf("[DEBUG] Uploading file: %s\n", displayName)
		fmt.Printf("[DEBUG] File size: %d bytes\n", fileInfo.Size())
		fmt.Printf("[DEBUG] MIME type: %s\n", mimeType)
	}

	// Read file content
	fileContent, err := io.ReadAll(file)
	if err != nil {
		return "", "", fmt.Errorf("failed to read file: %w", err)
	}

	// Upload using resumable upload API
	uploadURL := fmt.Sprintf("%s/upload/v1beta/files?key=%s", g.baseURL, g.apiKey)

	// Create the initial request to get upload URI
	initReqBody := map[string]interface{}{
		"file": map[string]string{
			"display_name": displayName,
		},
	}
	initJSON, _ := json.Marshal(initReqBody)

	initReq, err := http.NewRequestWithContext(ctx, "POST", uploadURL, bytes.NewBuffer(initJSON))
	if err != nil {
		return "", "", fmt.Errorf("failed to create init request: %w", err)
	}

	initReq.Header.Set("Content-Type", "application/json")
	initReq.Header.Set("X-Goog-Upload-Protocol", "resumable")
	initReq.Header.Set("X-Goog-Upload-Command", "start")
	initReq.Header.Set("X-Goog-Upload-Header-Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	initReq.Header.Set("X-Goog-Upload-Header-Content-Type", mimeType)

	if debug {
		fmt.Printf("[DEBUG] Init upload URL: %s\n", uploadURL)
	}

	initResp, err := g.client.Do(initReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to init upload: %w", err)
	}
	defer initResp.Body.Close()

	if initResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(initResp.Body)
		return "", "", fmt.Errorf("init upload failed: %s - %s", initResp.Status, string(body))
	}

	// Get the upload URI from response header
	resumableUploadURL := initResp.Header.Get("X-Goog-Upload-URL")
	if resumableUploadURL == "" {
		return "", "", fmt.Errorf("no upload URL in response")
	}

	if debug {
		fmt.Printf("[DEBUG] Resumable upload URL: %s\n", resumableUploadURL)
	}

	// Upload the actual file content
	uploadReq, err := http.NewRequestWithContext(ctx, "POST", resumableUploadURL, bytes.NewBuffer(fileContent))
	if err != nil {
		return "", "", fmt.Errorf("failed to create upload request: %w", err)
	}

	uploadReq.Header.Set("Content-Type", mimeType)
	uploadReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(fileContent)))
	uploadReq.Header.Set("X-Goog-Upload-Command", "upload, finalize")
	uploadReq.Header.Set("X-Goog-Upload-Offset", "0")

	uploadResp, err := g.client.Do(uploadReq)
	if err != nil {
		return "", "", fmt.Errorf("failed to upload file: %w", err)
	}
	defer uploadResp.Body.Close()

	uploadBody, err := io.ReadAll(uploadResp.Body)
	if err != nil {
		return "", "", fmt.Errorf("failed to read upload response: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Upload response status: %s\n", uploadResp.Status)
		fmt.Printf("[DEBUG] Upload response body: %s\n", string(uploadBody))
	}

	if uploadResp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("upload failed: %s - %s", uploadResp.Status, string(uploadBody))
	}

	var uploadResult geminiUploadInitResponse
	if err := json.Unmarshal(uploadBody, &uploadResult); err != nil {
		return "", "", fmt.Errorf("failed to parse upload response: %w", err)
	}

	return uploadResult.File.URI, mimeType, nil
}

// waitForFileProcessing polls until the file is ready
func (g *GeminiClient) waitForFileProcessing(ctx context.Context, fileURI string) error {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	// Extract the file name from URI (format: "https://generativelanguage.googleapis.com/v1beta/files/xxx")
	parts := strings.Split(fileURI, "/")
	if len(parts) < 2 {
		return fmt.Errorf("invalid file URI: %s", fileURI)
	}
	fileName := parts[len(parts)-1]

	statusURL := fmt.Sprintf("%s/v1beta/files/%s?key=%s", g.baseURL, fileName, g.apiKey)

	if debug {
		fmt.Printf("[DEBUG] Checking file status at: %s\n", statusURL)
	}

	// Poll for up to 5 minutes
	timeout := time.After(5 * time.Minute)
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for file processing")
		case <-ticker.C:
			req, err := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
			if err != nil {
				return fmt.Errorf("failed to create status request: %w", err)
			}

			resp, err := g.client.Do(req)
			if err != nil {
				if debug {
					fmt.Printf("[DEBUG] Status check failed: %v\n", err)
				}
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			if debug {
				fmt.Printf("[DEBUG] Status response: %s\n", string(body))
			}

			var status geminiFileStatusResponse
			if err := json.Unmarshal(body, &status); err != nil {
				continue
			}

			if status.Error != nil {
				return fmt.Errorf("file processing error: %s", status.Error.Message)
			}

			switch status.State {
			case "ACTIVE":
				return nil // File is ready
			case "FAILED":
				return fmt.Errorf("file processing failed")
			case "PROCESSING":
				if debug {
					fmt.Printf("[DEBUG] File still processing...\n")
				}
				// Continue polling
			}
		}
	}
}

// generateTranscript calls Gemini to generate a transcript from the video
func (g *GeminiClient) generateTranscript(ctx context.Context, fileURI, mimeType string) (*TranscriptResult, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	prompt := `Please transcribe all spoken content in this video. 
Provide the transcript in plain text format.
If there are multiple speakers, indicate speaker changes.
Include timestamps in [MM:SS] format at natural breaks in the conversation.
If there is no speech, indicate that the video has no spoken content.`

	reqBody := geminiGenerateRequest{
		Contents: []geminiContent{
			{
				Parts: []geminiPart{
					{
						FileData: &geminiFileData{
							MimeType: mimeType,
							FileURI:  fileURI,
						},
					},
					{
						Text: prompt,
					},
				},
			},
		},
		GenerationConfig: &geminiGenerateConfig{
			Temperature:     0.1,
			MaxOutputTokens: 8192,
		},
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	apiURL := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", g.baseURL, g.model, g.apiKey)

	if debug {
		fmt.Printf("[DEBUG] Generate URL: %s\n", apiURL)
		fmt.Printf("[DEBUG] Request body: %s\n", string(jsonBody))
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := g.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if debug {
		fmt.Printf("[DEBUG] Response status: %s\n", resp.Status)
		fmt.Printf("[DEBUG] Response body: %s\n", string(body))
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API request failed: %s - %s", resp.Status, string(body))
	}

	var genResp geminiGenerateResponse
	if err := json.Unmarshal(body, &genResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if genResp.Error != nil {
		return nil, fmt.Errorf("API error: %s", genResp.Error.Message)
	}

	if len(genResp.Candidates) == 0 || len(genResp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("no transcript generated")
	}

	transcript := genResp.Candidates[0].Content.Parts[0].Text

	return &TranscriptResult{
		Text: transcript,
	}, nil
}

// CheckGeminiConfig validates that Gemini is configured
func CheckGeminiConfig() error {
	if os.Getenv("GEMINI_API_KEY") == "" {
		return fmt.Errorf("GEMINI_API_KEY not set")
	}
	return nil
}

// GetGeminiHelp returns help text for setting up Gemini
func GetGeminiHelp() string {
	return `To use video transcription, you need a Google Gemini API key.

Setup:
  1. Go to https://aistudio.google.com/apikey
  2. Create an API key
  3. Set the environment variable:
     
     export GEMINI_API_KEY="your-api-key"
     
  Or add it to your .env file.

Optional:
  GEMINI_MODEL  - Model to use (default: gemini-2.0-flash)
                  Options: gemini-2.0-flash, gemini-1.5-pro, gemini-1.5-flash`
}
