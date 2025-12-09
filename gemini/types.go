// Package gemini provides a client for the Google Gemini API for image-to-markdown transcription.
// It also supports OpenAI-compatible APIs (LM Studio, Ollama, etc.) for local LLM transcription.
package gemini

import (
	"time"
)

// Provider type for transcription backend
type Provider string

const (
	// ProviderGemini uses Google's Gemini API
	ProviderGemini Provider = "gemini"
	// ProviderLocal uses local LLM via OpenAI-compatible APIs (LM Studio, Ollama, etc.)
	ProviderLocal Provider = "local"
	// ProviderAzureAnthropic uses Azure Anthropic (Claude) API
	ProviderAzureAnthropic Provider = "azure_anthropic"
)

// Model constants for Gemini models
const (
	// ModelGemini3Pro is the latest and most capable model (released Nov 2025)
	// Best for complex reasoning, agentic coding, and multimodal understanding
	ModelGemini3Pro = "gemini-3-pro-preview"
	// ModelGemini3ProThinking is Gemini 3 Pro with enhanced reasoning/thinking
	ModelGemini3ProThinking = "gemini-3-pro-preview-11-2025-thinking"
	// ModelGemini25Flash is the fast, efficient model for most tasks
	ModelGemini25Flash = "gemini-2.5-flash-preview-05-20"
	// ModelGemini25Pro is Gemini 2.5 Pro for complex reasoning
	ModelGemini25Pro = "gemini-2.5-pro-preview-05-06"
	// ModelGemini20Flash is the previous generation fast model
	ModelGemini20Flash = "gemini-2.0-flash"
)

// SupportedImageTypes lists all supported image file extensions
var SupportedImageTypes = []string{
	".jpg", ".jpeg", ".png", ".gif", ".webp", ".bmp", ".tiff", ".tif",
}

// ImageInfo contains metadata about an image file
type ImageInfo struct {
	Path      string
	Filename  string
	Size      int64
	Width     int
	Height    int
	MIMEType  string
	PageIndex int // Index in a multi-page document (0-based)
}

// TranscribeRequest configures an image transcription request
type TranscribeRequest struct {
	// Images is a list of image file paths to transcribe
	Images []string

	// OutputDir is the directory to write markdown files to
	OutputDir string

	// Model specifies which Gemini model to use
	Model string

	// Language specifies the language of the text (e.g., "en", "es", "fr")
	// Leave empty for auto-detection
	Language string

	// DetectChapters enables automatic chapter/section detection
	DetectChapters bool

	// PreserveFormatting attempts to preserve original document formatting
	PreserveFormatting bool

	// IncludeImageDescriptions adds descriptions for non-text images
	IncludeImageDescriptions bool

	// CombinePages combines all pages into a single markdown file when false chapter detection is used
	CombinePages bool

	// MaxTokensPerRequest limits tokens per API call (for rate limiting)
	MaxTokensPerRequest int

	// Temperature controls randomness (0.0-2.0, lower = more deterministic)
	Temperature *float64
}

// TranscribeResponse contains the transcription results
type TranscribeResponse struct {
	// Documents contains the generated markdown documents
	Documents []*MarkdownDocument

	// TotalPages is the total number of pages/images processed
	TotalPages int

	// ProcessingTime is the total time taken
	ProcessingTime time.Duration

	// TokensUsed is the total tokens consumed
	TokensUsed int
}

// MarkdownDocument represents a generated markdown file
type MarkdownDocument struct {
	// Filename is the suggested filename (without path)
	Filename string

	// Title is the document/chapter title
	Title string

	// Content is the markdown content
	Content string

	// PageRange indicates which pages are included
	PageRange PageRange

	// Sections contains detected sections within the document
	Sections []*Section

	// Metadata contains additional document metadata
	Metadata *DocumentMetadata
}

// PageRange represents a range of pages
type PageRange struct {
	Start int // 1-based page number
	End   int // 1-based page number (inclusive)
}

// Section represents a detected section within a document
type Section struct {
	// Title is the section heading
	Title string

	// Level is the heading level (1-6)
	Level int

	// StartPage is where this section begins (1-based)
	StartPage int

	// Content is the section's markdown content
	Content string
}

// DocumentMetadata contains extracted document information
type DocumentMetadata struct {
	// Title is the detected document title
	Title string

	// Author is the detected author if present
	Author string

	// Date is any detected date
	Date string

	// Language is the detected language
	Language string

	// Keywords are extracted keywords/topics
	Keywords []string

	// TableOfContents is the detected TOC if present
	TableOfContents []TOCEntry
}

// TOCEntry represents an entry in a table of contents
type TOCEntry struct {
	Title    string
	Level    int
	PageNum  int
	Children []TOCEntry
}

// PageContent represents the extracted content from a single page
type PageContent struct {
	PageNumber     int
	Text           string
	HasHeading     bool
	HeadingText    string
	HeadingLevel   int
	IsChapterStart bool
	ChapterTitle   string
	Images         []ImageDescription
}

// ImageDescription describes a non-text image on a page
type ImageDescription struct {
	Description string
	Type        string // "figure", "chart", "photo", "diagram", etc.
	Caption     string
}

// ChapterInfo represents a detected chapter boundary
type ChapterInfo struct {
	Title     string
	StartPage int
	EndPage   int
	Level     int // 1 = main chapter, 2 = sub-chapter, etc.
}

// APIError represents an error from the Gemini API
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Details    string `json:"details,omitempty"`
}

func (e *APIError) Error() string {
	if e.Details != "" {
		return e.Message + ": " + e.Details
	}
	return e.Message
}

// GenerateContentRequest is the request structure for the Gemini API
type GenerateContentRequest struct {
	Contents          []*Content        `json:"contents"`
	GenerationConfig  *GenerationConfig `json:"generationConfig,omitempty"`
	SafetySettings    []*SafetySetting  `json:"safetySettings,omitempty"`
	SystemInstruction *Content          `json:"systemInstruction,omitempty"`
}

// Content represents a content block in the API
type Content struct {
	Role  string  `json:"role,omitempty"`
	Parts []*Part `json:"parts"`
}

// Part represents a part of content (text or inline data)
type Part struct {
	Text       string      `json:"text,omitempty"`
	InlineData *InlineData `json:"inlineData,omitempty"`
}

// InlineData represents binary data (images) inline
type InlineData struct {
	MIMEType string `json:"mimeType"`
	Data     string `json:"data"` // Base64 encoded
}

// GenerationConfig contains generation parameters
type GenerationConfig struct {
	Temperature      *float64 `json:"temperature,omitempty"`
	TopP             *float64 `json:"topP,omitempty"`
	TopK             *int     `json:"topK,omitempty"`
	MaxOutputTokens  *int     `json:"maxOutputTokens,omitempty"`
	StopSequences    []string `json:"stopSequences,omitempty"`
	ResponseMimeType string   `json:"responseMimeType,omitempty"`
}

// SafetySetting configures content safety filters
type SafetySetting struct {
	Category  string `json:"category"`
	Threshold string `json:"threshold"`
}

// GenerateContentResponse is the response from the Gemini API
type GenerateContentResponse struct {
	Candidates    []*Candidate   `json:"candidates"`
	UsageMetadata *UsageMetadata `json:"usageMetadata,omitempty"`
}

// Candidate represents a generated response candidate
type Candidate struct {
	Content       *Content        `json:"content"`
	FinishReason  string          `json:"finishReason"`
	SafetyRatings []*SafetyRating `json:"safetyRatings,omitempty"`
}

// SafetyRating represents a content safety rating
type SafetyRating struct {
	Category    string `json:"category"`
	Probability string `json:"probability"`
}

// UsageMetadata contains token usage information
type UsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

// ============================================
// Local LLM types (LM Studio, Ollama - OpenAI-compatible API)
// ============================================

// LocalLLMMessage represents a message for local LLM
type LocalLLMMessage struct {
	Role    string            `json:"role"`
	Content []LocalLLMContent `json:"content,omitempty"`
}

// LocalLLMContent represents content for local LLM (text or image)
type LocalLLMContent struct {
	Type     string            `json:"type"`
	Text     string            `json:"text,omitempty"`
	ImageURL *LocalLLMImageURL `json:"image_url,omitempty"`
}

// LocalLLMImageURL represents an image URL for local LLM
type LocalLLMImageURL struct {
	URL    string `json:"url"`
	Detail string `json:"detail,omitempty"`
}

// LocalLLMRequest is the request structure for local LLM
type LocalLLMRequest struct {
	Model       string            `json:"model"`
	Messages    []LocalLLMMessage `json:"messages"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
}

// LocalLLMResponse is the response structure from local LLM
type LocalLLMResponse struct {
	ID      string           `json:"id"`
	Choices []LocalLLMChoice `json:"choices"`
	Usage   *LocalLLMUsage   `json:"usage,omitempty"`
	Error   *LocalLLMError   `json:"error,omitempty"`
}

// LocalLLMChoice represents a choice in the local LLM response
type LocalLLMChoice struct {
	Index        int                   `json:"index"`
	Message      LocalLLMChoiceMessage `json:"message"`
	FinishReason string                `json:"finish_reason"`
}

// LocalLLMChoiceMessage represents the message in a choice
type LocalLLMChoiceMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// LocalLLMUsage contains token usage information for local LLM
type LocalLLMUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// LocalLLMError represents an error from local LLM
type LocalLLMError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ============================================
// Progress callback types for UI updates
// ============================================

// ProgressStatus represents the current status of AI processing
type ProgressStatus int

const (
	StatusIdle ProgressStatus = iota
	StatusConnecting
	StatusSendingRequest
	StatusWaitingResponse
	StatusParsingResponse
	StatusProcessingBatch
	StatusRefining
	StatusComplete
	StatusError
)

// String returns a human-readable status description
func (s ProgressStatus) String() string {
	switch s {
	case StatusIdle:
		return "Ready"
	case StatusConnecting:
		return "Connecting to AI"
	case StatusSendingRequest:
		return "Sending request"
	case StatusWaitingResponse:
		return "Waiting for response"
	case StatusParsingResponse:
		return "Parsing response"
	case StatusProcessingBatch:
		return "Processing batch"
	case StatusRefining:
		return "Refining with text model"
	case StatusComplete:
		return "Complete"
	case StatusError:
		return "Error"
	default:
		return "Unknown"
	}
}

// ProgressUpdate contains information about AI processing progress
type ProgressUpdate struct {
	// Status is the current processing status
	Status ProgressStatus

	// Provider is the AI provider being used (gemini, local, azure_anthropic)
	Provider string

	// Model is the model name being used
	Model string

	// Message is a human-readable status message
	Message string

	// Detail provides additional context (e.g., "Image 3 of 10")
	Detail string

	// Progress is the overall progress (0.0 to 1.0)
	Progress float64

	// CurrentBatch is the current batch number (1-based)
	CurrentBatch int

	// TotalBatches is the total number of batches
	TotalBatches int

	// CurrentImage is the current image being processed (1-based)
	CurrentImage int

	// TotalImages is the total number of images
	TotalImages int

	// TokensUsed is the total tokens consumed so far
	TokensUsed int

	// Elapsed is the time elapsed since processing started
	Elapsed time.Duration

	// Stage indicates the pipeline stage (1 = vision, 2 = text refinement)
	Stage int

	// TotalStages is the total number of stages (1 or 2)
	TotalStages int

	// Error contains any error that occurred
	Error error

	// === Transparency fields for AI request/response logging ===

	// RequestInfo contains details about the AI request being made
	RequestInfo *AIRequestInfo

	// ResponseInfo contains details about the AI response received
	ResponseInfo *AIResponseInfo
}

// AIRequestInfo contains transparency details about an AI request
type AIRequestInfo struct {
	// Endpoint URL (sanitized, no API keys)
	Endpoint string

	// Method HTTP method (POST, GET, etc.)
	Method string

	// ContentType of the request
	ContentType string

	// DataSummary describes what data is being sent
	DataSummary string

	// ImageCount for image transcription requests
	ImageCount int

	// TotalDataSize in bytes
	TotalDataSize int64

	// PromptPreview shows first N characters of the prompt
	PromptPreview string

	// Parameters like temperature, max_tokens, etc.
	Parameters map[string]string
}

// AIResponseInfo contains transparency details about an AI response
type AIResponseInfo struct {
	// StatusCode HTTP status code
	StatusCode int

	// StatusText HTTP status text
	StatusText string

	// Latency time taken for the request
	Latency time.Duration

	// TokensInput consumed
	TokensInput int

	// TokensOutput generated
	TokensOutput int

	// TokensTotal consumed
	TokensTotal int

	// ContentPreview shows first N characters of response content
	ContentPreview string

	// ItemsProcessed (pages, clips, etc.)
	ItemsProcessed int

	// ErrorMessage if any
	ErrorMessage string
}

// ProgressCallback is a function called with progress updates during processing
type ProgressCallback func(update ProgressUpdate)

// TranscribeRequestWithProgress extends TranscribeRequest with progress callback
type TranscribeRequestWithProgress struct {
	*TranscribeRequest
	OnProgress ProgressCallback
}
