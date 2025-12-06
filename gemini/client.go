package gemini

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// BaseURL is the Google AI Studio API base URL
	BaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// DefaultTimeout for API requests
	DefaultTimeout = 5 * time.Minute

	// MaxImagesPerRequest is the maximum images per API call
	MaxImagesPerRequest = 16

	// MaxFileSize is the maximum file size per image (20MB)
	MaxFileSize = 20 * 1024 * 1024

	// MaxTotalImages is the maximum number of images per transcription job
	MaxTotalImages = 100
)

// Client is the Google Gemini API client
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	debug      bool
}

// ClientOption configures the Client
type ClientOption func(*Client)

// WithBaseURL sets a custom base URL (for testing)
func WithBaseURL(baseURL string) ClientOption {
	return func(c *Client) {
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return
		}
		if parsed.Scheme != "http" && parsed.Scheme != "https" {
			return
		}
		if parsed.Host == "" {
			return
		}
		c.baseURL = strings.TrimSuffix(baseURL, "/")
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

// NewClient creates a new Google Gemini API client
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

// NewClientFromEnv creates a client using the GEMINI_API_KEY or GOOGLE_API_KEY environment variable
func NewClientFromEnv(opts ...ClientOption) (*Client, error) {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY environment variable not set")
	}
	return NewClient(apiKey, opts...)
}

// TranscribeImages transcribes a set of images to markdown
func (c *Client) TranscribeImages(ctx context.Context, req *TranscribeRequest) (*TranscribeResponse, error) {
	startTime := time.Now()

	// Validate request
	if len(req.Images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}
	if len(req.Images) > MaxTotalImages {
		return nil, fmt.Errorf("maximum %d images allowed, got %d", MaxTotalImages, len(req.Images))
	}

	// Validate all images exist and are valid
	imageInfos := make([]*ImageInfo, 0, len(req.Images))
	for i, imgPath := range req.Images {
		info, err := c.getImageInfo(imgPath)
		if err != nil {
			return nil, fmt.Errorf("image %d (%s): %w", i+1, imgPath, err)
		}
		info.PageIndex = i
		imageInfos = append(imageInfos, info)
	}

	// Set defaults
	model := req.Model
	if model == "" {
		model = ModelGemini25Flash
	}

	// Process images in batches
	var allPageContents []*PageContent
	var totalTokens int

	for i := 0; i < len(imageInfos); i += MaxImagesPerRequest {
		end := i + MaxImagesPerRequest
		if end > len(imageInfos) {
			end = len(imageInfos)
		}

		batch := imageInfos[i:end]
		pageContents, tokens, err := c.processBatch(ctx, batch, req, model)
		if err != nil {
			return nil, fmt.Errorf("batch starting at page %d: %w", i+1, err)
		}

		allPageContents = append(allPageContents, pageContents...)
		totalTokens += tokens
	}

	// Detect chapters and organize content
	var documents []*MarkdownDocument
	if req.DetectChapters {
		documents = c.organizeByChapters(allPageContents, req)
	} else if req.CombinePages {
		documents = []*MarkdownDocument{c.combineAllPages(allPageContents, req)}
	} else {
		documents = c.createPerPageDocuments(allPageContents, req)
	}

	return &TranscribeResponse{
		Documents:      documents,
		TotalPages:     len(req.Images),
		ProcessingTime: time.Since(startTime),
		TokensUsed:     totalTokens,
	}, nil
}

// getImageInfo validates and extracts info about an image file
func (c *Client) getImageInfo(path string) (*ImageInfo, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access file: %w", err)
	}

	if info.IsDir() {
		return nil, fmt.Errorf("path is a directory, not a file")
	}

	if info.Size() > MaxFileSize {
		return nil, fmt.Errorf("file size %d exceeds maximum %d bytes (20MB)", info.Size(), MaxFileSize)
	}

	ext := strings.ToLower(filepath.Ext(path))
	mimeType := getMIMEType(ext)
	if mimeType == "" {
		return nil, fmt.Errorf("unsupported image format: %s", ext)
	}

	return &ImageInfo{
		Path:     path,
		Filename: filepath.Base(path),
		Size:     info.Size(),
		MIMEType: mimeType,
	}, nil
}

// getMIMEType returns the MIME type for an image extension
func getMIMEType(ext string) string {
	switch ext {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".bmp":
		return "image/bmp"
	case ".tiff", ".tif":
		return "image/tiff"
	default:
		return ""
	}
}

// processBatch processes a batch of images
func (c *Client) processBatch(ctx context.Context, images []*ImageInfo, req *TranscribeRequest, model string) ([]*PageContent, int, error) {
	// Build the prompt
	prompt := c.buildExtractionPrompt(images, req)

	// Build parts with images
	parts := make([]*Part, 0, len(images)+1)

	// Add images first
	for _, img := range images {
		data, err := os.ReadFile(img.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to read %s: %w", img.Filename, err)
		}

		parts = append(parts, &Part{
			InlineData: &InlineData{
				MIMEType: img.MIMEType,
				Data:     base64.StdEncoding.EncodeToString(data),
			},
		})
	}

	// Add prompt text
	parts = append(parts, &Part{Text: prompt})

	// Build request
	apiReq := &GenerateContentRequest{
		Contents: []*Content{
			{
				Role:  "user",
				Parts: parts,
			},
		},
		GenerationConfig: &GenerationConfig{
			MaxOutputTokens: intPtr(8192),
			ResponseMimeType: "application/json",
		},
	}

	if req.Temperature != nil {
		apiReq.GenerationConfig.Temperature = req.Temperature
	}

	// Make API call
	resp, err := c.generateContent(ctx, model, apiReq)
	if err != nil {
		return nil, 0, err
	}

	// Parse response
	pageContents, err := c.parseExtractionResponse(resp, images)
	if err != nil {
		return nil, 0, err
	}

	tokens := 0
	if resp.UsageMetadata != nil {
		tokens = resp.UsageMetadata.TotalTokenCount
	}

	return pageContents, tokens, nil
}

// buildExtractionPrompt creates the prompt for text extraction
func (c *Client) buildExtractionPrompt(images []*ImageInfo, req *TranscribeRequest) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert OCR and document analysis system. Extract all text content from the provided images, which are pages from a document.

For each page/image, output a JSON object with the following structure:
{
  "pages": [
    {
      "page_number": 1,
      "text": "The full text content in markdown format",
      "has_heading": true,
      "heading_text": "Chapter 1: Introduction",
      "heading_level": 1,
      "is_chapter_start": true,
      "chapter_title": "Introduction",
      "images": [
        {"description": "A bar chart showing sales data", "type": "chart", "caption": "Figure 1.1"}
      ]
    }
  ]
}

Guidelines:
1. Preserve the original formatting as much as possible using markdown:
   - Use # for headings (# for h1, ## for h2, etc.)
   - Use **bold** and *italic* where appropriate
   - Use bullet points and numbered lists
   - Use > for blockquotes
   - Use code blocks for code/preformatted text
   - Use tables for tabular data (markdown table format)

2. Chapter/Section Detection:
   - Identify chapter starts (large headings, "Chapter X", "Part X", etc.)
   - Note section headings and their hierarchy
   - Mark page breaks between logical sections

3. Image Descriptions:
   - For figures, charts, diagrams, photos - provide brief descriptions
   - Include captions if visible
   - Classify the image type (figure, chart, photo, diagram, table, etc.)

`)

	if req.Language != "" {
		sb.WriteString(fmt.Sprintf("4. The document is in %s. Output the text in the same language.\n\n", req.Language))
	} else {
		sb.WriteString("4. Auto-detect the document language and preserve it in the output.\n\n")
	}

	if req.PreserveFormatting {
		sb.WriteString("5. Pay special attention to preserving:\n")
		sb.WriteString("   - Paragraph structure and spacing\n")
		sb.WriteString("   - Indentation levels\n")
		sb.WriteString("   - Special characters and symbols\n")
		sb.WriteString("   - Mathematical notation (use LaTeX format: $equation$)\n\n")
	}

	sb.WriteString(fmt.Sprintf("Process the following %d images as consecutive pages (starting from page %d):\n",
		len(images), images[0].PageIndex+1))

	return sb.String()
}

// generateContent makes an API call to generate content
func (c *Client) generateContent(ctx context.Context, model string, req *GenerateContentRequest) (*GenerateContentResponse, error) {
	// Build URL
	apiURL := fmt.Sprintf("%s/models/%s:generateContent?key=%s", c.baseURL, model, c.apiKey)

	// Serialize request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.debug {
		fmt.Printf("[DEBUG] POST %s\n", strings.Replace(apiURL, c.apiKey, "***", 1))
		// Don't log the full body as it contains large base64 images
		fmt.Printf("[DEBUG] Request parts: %d\n", len(req.Contents[0].Parts))
	}

	// Create HTTP request
	httpReq, err := http.NewRequestWithContext(ctx, "POST", apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")

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
		var apiErr struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
				Status  string `json:"status"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, string(respBody))
		}
		return nil, &APIError{
			StatusCode: resp.StatusCode,
			Message:    apiErr.Error.Message,
			Details:    apiErr.Error.Status,
		}
	}

	// Parse response
	var result GenerateContentResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &result, nil
}

// parseExtractionResponse parses the API response into page contents
func (c *Client) parseExtractionResponse(resp *GenerateContentResponse, images []*ImageInfo) ([]*PageContent, error) {
	if len(resp.Candidates) == 0 {
		return nil, fmt.Errorf("no candidates in response")
	}

	candidate := resp.Candidates[0]
	if candidate.Content == nil || len(candidate.Content.Parts) == 0 {
		return nil, fmt.Errorf("no content in response")
	}

	// Get the text content
	text := candidate.Content.Parts[0].Text

	// Try to parse as JSON
	var result struct {
		Pages []*PageContent `json:"pages"`
	}

	// Clean up the text (remove markdown code blocks if present)
	text = strings.TrimSpace(text)
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// If JSON parsing fails, treat the whole response as a single page
		if c.debug {
			fmt.Printf("[DEBUG] JSON parse failed, treating as raw text: %v\n", err)
		}
		return []*PageContent{
			{
				PageNumber: images[0].PageIndex + 1,
				Text:       text,
			},
		}, nil
	}

	// Adjust page numbers to match actual indices
	for i, page := range result.Pages {
		if i < len(images) {
			page.PageNumber = images[i].PageIndex + 1
		}
	}

	return result.Pages, nil
}

// organizeByChapters groups pages into chapters
func (c *Client) organizeByChapters(pages []*PageContent, req *TranscribeRequest) []*MarkdownDocument {
	if len(pages) == 0 {
		return nil
	}

	// Find chapter boundaries
	chapters := c.detectChapters(pages)

	if len(chapters) == 0 {
		// No chapters detected, create single document
		return []*MarkdownDocument{c.combineAllPages(pages, req)}
	}

	// Create documents for each chapter
	documents := make([]*MarkdownDocument, 0, len(chapters))

	for i, chapter := range chapters {
		// Get pages for this chapter
		var chapterPages []*PageContent
		for _, page := range pages {
			if page.PageNumber >= chapter.StartPage && page.PageNumber <= chapter.EndPage {
				chapterPages = append(chapterPages, page)
			}
		}

		if len(chapterPages) == 0 {
			continue
		}

		doc := c.createDocumentFromPages(chapterPages, chapter.Title, i+1, req)
		doc.PageRange = PageRange{Start: chapter.StartPage, End: chapter.EndPage}
		documents = append(documents, doc)
	}

	return documents
}

// detectChapters finds chapter boundaries in the pages
func (c *Client) detectChapters(pages []*PageContent) []*ChapterInfo {
	var chapters []*ChapterInfo

	for i, page := range pages {
		if page.IsChapterStart || (page.HasHeading && page.HeadingLevel <= 2) {
			title := page.ChapterTitle
			if title == "" {
				title = page.HeadingText
			}
			if title == "" {
				title = fmt.Sprintf("Section %d", len(chapters)+1)
			}

			// Close previous chapter
			if len(chapters) > 0 {
				chapters[len(chapters)-1].EndPage = page.PageNumber - 1
			}

			chapters = append(chapters, &ChapterInfo{
				Title:     title,
				StartPage: page.PageNumber,
				EndPage:   pages[len(pages)-1].PageNumber, // Will be updated
				Level:     page.HeadingLevel,
			})
		}

		// Update end page for last chapter
		if len(chapters) > 0 && i == len(pages)-1 {
			chapters[len(chapters)-1].EndPage = page.PageNumber
		}
	}

	return chapters
}

// combineAllPages creates a single document from all pages
func (c *Client) combineAllPages(pages []*PageContent, req *TranscribeRequest) *MarkdownDocument {
	return c.createDocumentFromPages(pages, "Document", 0, req)
}

// createDocumentFromPages creates a markdown document from a set of pages
func (c *Client) createDocumentFromPages(pages []*PageContent, title string, index int, req *TranscribeRequest) *MarkdownDocument {
	var content strings.Builder
	var sections []*Section

	// Add title if specified
	if title != "" && title != "Document" {
		content.WriteString(fmt.Sprintf("# %s\n\n", title))
	}

	for i, page := range pages {
		// Add page separator if not first page
		if i > 0 {
			content.WriteString("\n\n---\n\n")
		}

		// Track sections
		if page.HasHeading && page.HeadingLevel > 0 {
			sections = append(sections, &Section{
				Title:     page.HeadingText,
				Level:     page.HeadingLevel,
				StartPage: page.PageNumber,
			})
		}

		// Add page content
		content.WriteString(page.Text)

		// Add image descriptions if enabled
		if req.IncludeImageDescriptions && len(page.Images) > 0 {
			content.WriteString("\n\n")
			for _, img := range page.Images {
				content.WriteString(fmt.Sprintf("*[%s: %s", img.Type, img.Description))
				if img.Caption != "" {
					content.WriteString(fmt.Sprintf(" - %s", img.Caption))
				}
				content.WriteString("]*\n\n")
			}
		}
	}

	// Generate filename
	filename := sanitizeFilename(title)
	if index > 0 {
		filename = fmt.Sprintf("%02d_%s", index, filename)
	}
	filename += ".md"

	return &MarkdownDocument{
		Filename: filename,
		Title:    title,
		Content:  content.String(),
		PageRange: PageRange{
			Start: pages[0].PageNumber,
			End:   pages[len(pages)-1].PageNumber,
		},
		Sections: sections,
	}
}

// createPerPageDocuments creates one document per page
func (c *Client) createPerPageDocuments(pages []*PageContent, req *TranscribeRequest) []*MarkdownDocument {
	documents := make([]*MarkdownDocument, 0, len(pages))

	for _, page := range pages {
		doc := &MarkdownDocument{
			Filename: fmt.Sprintf("page_%03d.md", page.PageNumber),
			Title:    fmt.Sprintf("Page %d", page.PageNumber),
			Content:  page.Text,
			PageRange: PageRange{
				Start: page.PageNumber,
				End:   page.PageNumber,
			},
		}

		if page.HasHeading {
			doc.Title = page.HeadingText
			doc.Sections = []*Section{
				{
					Title:     page.HeadingText,
					Level:     page.HeadingLevel,
					StartPage: page.PageNumber,
				},
			}
		}

		// Add image descriptions
		if req.IncludeImageDescriptions && len(page.Images) > 0 {
			var imgContent strings.Builder
			imgContent.WriteString(doc.Content)
			imgContent.WriteString("\n\n")
			for _, img := range page.Images {
				imgContent.WriteString(fmt.Sprintf("*[%s: %s", img.Type, img.Description))
				if img.Caption != "" {
					imgContent.WriteString(fmt.Sprintf(" - %s", img.Caption))
				}
				imgContent.WriteString("]*\n\n")
			}
			doc.Content = imgContent.String()
		}

		documents = append(documents, doc)
	}

	return documents
}

// TranscribeWithRetry transcribes with automatic retry on transient failures
func (c *Client) TranscribeWithRetry(ctx context.Context, req *TranscribeRequest, maxRetries int) (*TranscribeResponse, error) {
	var lastErr error
	backoff := []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, err := c.TranscribeImages(ctx, req)
		if err == nil {
			return result, nil
		}

		lastErr = err

		// Check if error is retryable
		if apiErr, ok := err.(*APIError); ok {
			// Don't retry client errors (4xx) except rate limits
			if apiErr.StatusCode >= 400 && apiErr.StatusCode < 500 && apiErr.StatusCode != 429 {
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

// sanitizeFilename creates a safe filename from a title
func sanitizeFilename(title string) string {
	// Replace unsafe characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
		" ", "_",
	)
	result := replacer.Replace(title)

	// Remove consecutive underscores
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}

	// Trim underscores from ends
	result = strings.Trim(result, "_")

	// Limit length
	if len(result) > 50 {
		result = result[:50]
	}

	// Default name if empty
	if result == "" {
		result = "document"
	}

	return strings.ToLower(result)
}

// Helper function to create int pointer
func intPtr(i int) *int {
	return &i
}

// GetAPIKeyHelp returns help text for setting up the API key
func GetAPIKeyHelp() string {
	return `To use the Google Gemini API for image transcription, you need an API key.

1. Go to https://aistudio.google.com/apikey
2. Sign in with your Google account
3. Click "Create API key"
4. Copy the API key
5. Set the environment variable:

   export GEMINI_API_KEY="your-api-key"

Or create a .env file with:
   GEMINI_API_KEY=your-api-key

Pricing: https://ai.google.dev/gemini-api/docs/pricing
- Free tier: 60 requests per minute, 1500 requests per day
- Paid plans available for higher usage`
}

// CheckConfig verifies the Gemini configuration is set up
func CheckConfig() error {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("GEMINI_API_KEY or GOOGLE_API_KEY environment variable not set")
	}
	return nil
}
