// Package gemini provides a client for the Google Gemini API for image-to-markdown transcription.
package gemini

import (
	"time"
)

// Model constants for Gemini models
const (
	// ModelGemini25Flash is the fast, efficient model for most tasks
	ModelGemini25Flash = "gemini-2.5-flash-preview-05-20"
	// ModelGemini25Pro is the most capable model for complex reasoning
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
	PageNumber int
	Text       string
	HasHeading bool
	HeadingText string
	HeadingLevel int
	IsChapterStart bool
	ChapterTitle string
	Images []ImageDescription
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
	Contents         []*Content         `json:"contents"`
	GenerationConfig *GenerationConfig  `json:"generationConfig,omitempty"`
	SafetySettings   []*SafetySetting   `json:"safetySettings,omitempty"`
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
	Temperature     *float64 `json:"temperature,omitempty"`
	TopP            *float64 `json:"topP,omitempty"`
	TopK            *int     `json:"topK,omitempty"`
	MaxOutputTokens *int     `json:"maxOutputTokens,omitempty"`
	StopSequences   []string `json:"stopSequences,omitempty"`
	ResponseMimeType string  `json:"responseMimeType,omitempty"`
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
	Content       *Content       `json:"content"`
	FinishReason  string         `json:"finishReason"`
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
