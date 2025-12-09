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
	"sync"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

const (
	// BaseURL is the Google AI Studio API base URL
	BaseURL = "https://generativelanguage.googleapis.com/v1beta"

	// DefaultTimeout for API requests
	DefaultTimeout = 5 * time.Minute

	// MaxImagesPerRequest is the maximum images per API call (Gemini supports up to 3600)
	// We use a conservative limit for better performance and reliability
	MaxImagesPerRequest = 20

	// MaxFileSize is the maximum file size per image (20MB for inline, 2GB via File API)
	MaxFileSize = 20 * 1024 * 1024

	// MaxTotalImages is the maximum number of images per transcription job
	MaxTotalImages = 100

	// MaxInlineRequestSize is the official Gemini limit for inline data requests
	// Total request size including prompts, system instructions, and all base64 encoded images
	MaxInlineRequestSize = 20 * 1024 * 1024

	// MaxPayloadSize is our practical limit accounting for base64 overhead (~1.37x) and prompt text
	// 20MB limit / 1.4 overhead factor = ~14.3MB raw images, we use 14MB to be safe
	MaxPayloadSize = 14 * 1024 * 1024

	// MaxConcurrentRequests is the maximum parallel API calls
	// Free tier: 5 RPM, Tier 1: 500 RPM, Tier 2+: 1000+ RPM
	MaxConcurrentRequests = 3

	// GeminiContextWindow is the maximum input tokens (1M, 2M coming soon)
	GeminiContextWindow = 1000000

	// GeminiMaxOutputTokens is the maximum output tokens
	GeminiMaxOutputTokens = 65535
)

// Client is the Google Gemini API client (also supports OpenAI-compatible APIs and Azure Anthropic)
type Client struct {
	provider   Provider
	apiKey     string
	baseURL    string
	model      string // For OpenAI provider (vision model for local LLM)
	httpClient *http.Client
	debug      bool

	// Two-stage pipeline for local LLM (optional)
	// If set, vision model extracts raw text, then text model refines into markdown
	textModel    string // Agentic/text model for refinement (e.g., mistral, llama)
	textEndpoint string // Endpoint for text model (can be same or different server)

	// For Azure Anthropic provider
	anthropicClient *anthropic.Client
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

// WithProvider sets the provider type
func WithProvider(provider Provider) ClientOption {
	return func(c *Client) {
		c.provider = provider
	}
}

// WithModel sets the model name (mainly for OpenAI provider)
func WithModel(model string) ClientOption {
	return func(c *Client) {
		c.model = model
	}
}

// WithTextModel sets the text/agentic model for two-stage pipeline
func WithTextModel(model string) ClientOption {
	return func(c *Client) {
		c.textModel = model
	}
}

// WithTextEndpoint sets the endpoint for the text model (can be different from vision model)
func WithTextEndpoint(endpoint string) ClientOption {
	return func(c *Client) {
		c.textEndpoint = strings.TrimSuffix(endpoint, "/")
	}
}

// NewClient creates a new Google Gemini API client
func NewClient(apiKey string, opts ...ClientOption) (*Client, error) {
	if apiKey == "" {
		return nil, fmt.Errorf("API key is required")
	}

	c := &Client{
		provider: ProviderGemini,
		apiKey:   apiKey,
		baseURL:  BaseURL,
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

// NewLocalClient creates a client for local LLM (LM Studio, Ollama, etc.)
func NewLocalClient(endpoint, model string, opts ...ClientOption) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("LLM endpoint is required")
	}

	endpoint = strings.TrimSuffix(endpoint, "/")

	if model == "" {
		model = "local-model"
	}

	c := &Client{
		provider: ProviderLocal,
		baseURL:  endpoint,
		model:    model,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute, // Longer timeout for local models
		},
		debug: false,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// NewAzureAnthropicClient creates a client for Azure Anthropic (Claude)
func NewAzureAnthropicClient(endpoint, apiKey, model string, opts ...ClientOption) (*Client, error) {
	if endpoint == "" {
		return nil, fmt.Errorf("Azure Anthropic endpoint is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("Azure Anthropic API key is required")
	}

	endpoint = strings.TrimSuffix(endpoint, "/")

	if model == "" {
		model = "claude-sonnet-4-20250514" // Default to Claude Sonnet 4
	}

	// Create Anthropic client with Azure endpoint
	anthropicClient := anthropic.NewClient(
		option.WithAPIKey(apiKey),
		option.WithBaseURL(endpoint),
	)

	c := &Client{
		provider: ProviderAzureAnthropic,
		baseURL:  endpoint,
		apiKey:   apiKey,
		model:    model,
		httpClient: &http.Client{
			Timeout: 5 * time.Minute,
		},
		debug:           false,
		anthropicClient: &anthropicClient,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// IsTwoStageEnabled returns true if the client has a separate text model configured
func (c *Client) IsTwoStageEnabled() bool {
	return c.textModel != ""
}

// GetTextModel returns the configured text model name
func (c *Client) GetTextModel() string {
	return c.textModel
}

// NewClientFromEnv creates a client using environment variables
// It first checks for local LLM (LLM_ENDPOINT), then falls back to Gemini API
//
// For local LLM two-stage pipeline (vision + text model):
//   - IMAGE_VISION_MODEL / LLM_MODEL: Vision model for image scanning (e.g., llava, qwen-vl)
//   - IMAGE_TEXT_MODEL: Text/agentic model for markdown generation (e.g., mistral, llama)
//   - IMAGE_TEXT_ENDPOINT: Optional separate endpoint for text model
func NewClientFromEnv(opts ...ClientOption) (*Client, error) {
	debug := os.Getenv("CAPYCUT_DEBUG") != ""

	// Check for local LLM first (same env vars as video clipping)
	localEndpoint := os.Getenv("LLM_ENDPOINT")
	localModel := os.Getenv("LLM_MODEL")

	// Also check for image-specific overrides
	if imgEndpoint := os.Getenv("IMAGE_LLM_ENDPOINT"); imgEndpoint != "" {
		localEndpoint = imgEndpoint
	}
	if imgModel := os.Getenv("IMAGE_LLM_MODEL"); imgModel != "" {
		localModel = imgModel
	}
	// Vision model override (more explicit name)
	if visionModel := os.Getenv("IMAGE_VISION_MODEL"); visionModel != "" {
		localModel = visionModel
	}

	// Two-stage pipeline: separate text/agentic model for refinement
	textModel := os.Getenv("IMAGE_TEXT_MODEL")
	textEndpoint := os.Getenv("IMAGE_TEXT_ENDPOINT")
	if textEndpoint == "" {
		textEndpoint = localEndpoint // Default to same endpoint
	}

	if localEndpoint != "" {
		if debug {
			fmt.Println("\n[DEBUG] Local LLM Configuration (Image Transcription):")
			fmt.Printf("  Vision Endpoint: %s\n", localEndpoint)
			fmt.Printf("  Vision Model:    %s\n", localModel)
			if textModel != "" {
				fmt.Printf("  Text Endpoint:   %s\n", textEndpoint)
				fmt.Printf("  Text Model:      %s (two-stage pipeline enabled)\n", textModel)
			} else {
				fmt.Println("  Text Model:      (same as vision - single-stage)")
			}
			fmt.Println()
		}
		clientOpts := append(opts,
			WithTextModel(textModel),
			WithTextEndpoint(textEndpoint),
		)
		return NewLocalClient(localEndpoint, localModel, clientOpts...)
	}

	// Check for Azure Anthropic
	azureAnthropicEndpoint := os.Getenv("AZURE_ANTHROPIC_ENDPOINT")
	azureAnthropicAPIKey := os.Getenv("AZURE_ANTHROPIC_API_KEY")
	azureAnthropicModel := os.Getenv("AZURE_ANTHROPIC_MODEL")

	if azureAnthropicEndpoint != "" {
		if debug {
			fmt.Println("\n[DEBUG] Azure Anthropic Configuration (Image Transcription):")
			fmt.Printf("  Endpoint: %s\n", azureAnthropicEndpoint)
			if azureAnthropicAPIKey != "" {
				fmt.Printf("  API Key:  %s...%s\n", azureAnthropicAPIKey[:4], azureAnthropicAPIKey[len(azureAnthropicAPIKey)-4:])
			}
			fmt.Printf("  Model:    %s\n", azureAnthropicModel)
			fmt.Println()
		}
		return NewAzureAnthropicClient(azureAnthropicEndpoint, azureAnthropicAPIKey, azureAnthropicModel, opts...)
	}

	// Fall back to Gemini API
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("no backend configured. Set LLM_ENDPOINT for local LLM, AZURE_ANTHROPIC_ENDPOINT for Azure Anthropic, or GEMINI_API_KEY for Gemini")
	}
	return NewClient(apiKey, opts...)
}

// batchResult holds the result of processing a batch
type batchResult struct {
	batchIndex int
	pages      []*PageContent
	tokens     int
	err        error
}

// transcribeContext holds state during transcription
type transcribeContext struct {
	startTime    time.Time
	totalImages  int
	totalBatches int
	tokensUsed   int
	onProgress   ProgressCallback
}

// sendProgress sends a progress update if callback is configured
func (c *Client) sendProgress(ctx *transcribeContext, update ProgressUpdate) {
	if ctx == nil || ctx.onProgress == nil {
		return
	}

	// Fill in common fields
	update.TotalImages = ctx.totalImages
	update.TotalBatches = ctx.totalBatches
	update.TokensUsed = ctx.tokensUsed
	update.Elapsed = time.Since(ctx.startTime)

	// Set provider and model info
	update.Provider = string(c.provider)
	switch c.provider {
	case ProviderGemini:
		if update.Model == "" {
			update.Model = "Gemini"
		}
	case ProviderLocal:
		if update.Model == "" {
			update.Model = c.model
		}
	case ProviderAzureAnthropic:
		if update.Model == "" {
			update.Model = c.model
		}
	}

	// Calculate progress if not set
	if update.Progress == 0 && ctx.totalBatches > 0 && update.CurrentBatch > 0 {
		update.Progress = float64(update.CurrentBatch-1) / float64(ctx.totalBatches)
		if update.Status == StatusComplete {
			update.Progress = 1.0
		}
	}

	ctx.onProgress(update)
}

// TranscribeImages transcribes a set of images to markdown using smart batching
func (c *Client) TranscribeImages(ctx context.Context, req *TranscribeRequest) (*TranscribeResponse, error) {
	return c.TranscribeImagesWithProgress(ctx, req, nil)
}

// TranscribeImagesWithProgress transcribes images with progress callbacks for UI updates
func (c *Client) TranscribeImagesWithProgress(ctx context.Context, req *TranscribeRequest, onProgress ProgressCallback) (*TranscribeResponse, error) {
	startTime := time.Now()

	// Initialize progress tracking context
	tctx := &transcribeContext{
		startTime:  startTime,
		onProgress: onProgress,
	}

	// Send initial progress
	c.sendProgress(tctx, ProgressUpdate{
		Status:  StatusConnecting,
		Message: "Initializing transcription",
		Detail:  "Validating images...",
	})

	// Validate request
	if len(req.Images) == 0 {
		return nil, fmt.Errorf("at least one image is required")
	}
	if len(req.Images) > MaxTotalImages {
		return nil, fmt.Errorf("maximum %d images allowed, got %d", MaxTotalImages, len(req.Images))
	}

	tctx.totalImages = len(req.Images)

	// Validate all images exist and are valid
	imageInfos := make([]*ImageInfo, 0, len(req.Images))
	for i, imgPath := range req.Images {
		info, err := c.getImageInfo(imgPath)
		if err != nil {
			c.sendProgress(tctx, ProgressUpdate{
				Status:  StatusError,
				Message: "Image validation failed",
				Detail:  fmt.Sprintf("Image %d (%s): %v", i+1, imgPath, err),
				Error:   err,
			})
			return nil, fmt.Errorf("image %d (%s): %w", i+1, imgPath, err)
		}
		info.PageIndex = i
		imageInfos = append(imageInfos, info)
	}

	// Set defaults - use Gemini 3 Pro as default (most capable model)
	model := req.Model
	if model == "" {
		model = ModelGemini3Pro
	}

	// Create smart batches based on payload size
	// For local LLM, use much smaller batches (1 image at a time) due to context limits
	var batches [][]*ImageInfo
	if c.provider == ProviderLocal {
		batches = c.createLocalLLMBatches(imageInfos)
	} else {
		batches = c.createSmartBatches(imageInfos)
	}

	tctx.totalBatches = len(batches)

	// Determine number of stages
	totalStages := 1
	if c.textModel != "" {
		totalStages = 2
	}

	// Send progress update after batching
	c.sendProgress(tctx, ProgressUpdate{
		Status:      StatusProcessingBatch,
		Message:     fmt.Sprintf("Processing %d images in %d batches", len(imageInfos), len(batches)),
		Detail:      fmt.Sprintf("Using %s", c.getProviderDisplayName()),
		TotalStages: totalStages,
		Stage:       1,
		Model:       model,
	})

	if c.debug {
		fmt.Printf("[DEBUG] Created %d batches from %d images\n", len(batches), len(imageInfos))
		for i, batch := range batches {
			totalSize := int64(0)
			for _, img := range batch {
				totalSize += img.Size
			}
			fmt.Printf("[DEBUG] Batch %d: %d images, %.2f MB\n", i+1, len(batch), float64(totalSize)/(1024*1024))
		}
	}

	// Process batches in parallel with worker pool
	allPageContents, totalTokens, err := c.processBatchesParallelWithProgress(ctx, batches, req, model, tctx)
	if err != nil {
		c.sendProgress(tctx, ProgressUpdate{
			Status:  StatusError,
			Message: "Transcription failed",
			Detail:  err.Error(),
			Error:   err,
		})
		return nil, err
	}

	tctx.tokensUsed = totalTokens

	// Send completion progress
	c.sendProgress(tctx, ProgressUpdate{
		Status:       StatusComplete,
		Message:      "Transcription complete",
		Detail:       fmt.Sprintf("Processed %d images, %d tokens used", len(imageInfos), totalTokens),
		CurrentBatch: len(batches),
		Progress:     1.0,
	})

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

// getProviderDisplayName returns a user-friendly provider name
func (c *Client) getProviderDisplayName() string {
	switch c.provider {
	case ProviderGemini:
		return "Google Gemini"
	case ProviderLocal:
		return fmt.Sprintf("Local LLM (%s)", c.model)
	case ProviderAzureAnthropic:
		return fmt.Sprintf("Azure Anthropic (%s)", c.model)
	default:
		return string(c.provider)
	}
}

// createSmartBatches groups images into batches based on payload size limits
func (c *Client) createSmartBatches(images []*ImageInfo) [][]*ImageInfo {
	var batches [][]*ImageInfo
	var currentBatch []*ImageInfo
	var currentSize int64

	// Calculate base64 overhead factor (~1.37x for base64 encoding + JSON overhead)
	const overheadFactor = 1.4

	for _, img := range images {
		estimatedSize := int64(float64(img.Size) * overheadFactor)

		// Check if adding this image would exceed limits
		wouldExceedSize := currentSize+estimatedSize > MaxPayloadSize
		wouldExceedCount := len(currentBatch) >= MaxImagesPerRequest

		if (wouldExceedSize || wouldExceedCount) && len(currentBatch) > 0 {
			// Start a new batch
			batches = append(batches, currentBatch)
			currentBatch = nil
			currentSize = 0
		}

		// Handle case where single image exceeds payload size
		if estimatedSize > MaxPayloadSize {
			if c.debug {
				fmt.Printf("[DEBUG] Warning: Image %s (%.2f MB) is large, processing alone\n",
					img.Filename, float64(img.Size)/(1024*1024))
			}
			// Process this image alone in its own batch
			batches = append(batches, []*ImageInfo{img})
			continue
		}

		currentBatch = append(currentBatch, img)
		currentSize += estimatedSize
	}

	// Don't forget the last batch
	if len(currentBatch) > 0 {
		batches = append(batches, currentBatch)
	}

	return batches
}

// createLocalLLMBatches creates batches optimized for local LLM with limited context
// Local LLMs typically have much smaller context windows, so we process 1 image at a time
func (c *Client) createLocalLLMBatches(images []*ImageInfo) [][]*ImageInfo {
	batches := make([][]*ImageInfo, 0, len(images))

	for _, img := range images {
		// Each image gets its own batch to avoid context overflow
		batches = append(batches, []*ImageInfo{img})
	}

	if c.debug {
		fmt.Printf("[DEBUG] Local LLM mode: Processing %d images one at a time\n", len(images))
	}

	return batches
}

// processBatchesParallel processes batches concurrently with a worker pool
func (c *Client) processBatchesParallel(ctx context.Context, batches [][]*ImageInfo, req *TranscribeRequest, model string) ([]*PageContent, int, error) {
	return c.processBatchesParallelWithProgress(ctx, batches, req, model, nil)
}

// processBatchesParallelWithProgress processes batches with progress updates
func (c *Client) processBatchesParallelWithProgress(ctx context.Context, batches [][]*ImageInfo, req *TranscribeRequest, model string, tctx *transcribeContext) ([]*PageContent, int, error) {
	if len(batches) == 0 {
		return nil, 0, nil
	}

	// For small number of batches, process sequentially to avoid overhead
	if len(batches) <= 2 {
		return c.processBatchesSequentialWithProgress(ctx, batches, req, model, tctx)
	}

	// Process in parallel with worker pool
	results := make(chan *batchResult, len(batches))
	sem := make(chan struct{}, MaxConcurrentRequests)

	var wg sync.WaitGroup

	for i, batch := range batches {
		wg.Add(1)
		go func(idx int, imgs []*ImageInfo) {
			defer wg.Done()

			// Acquire semaphore
			sem <- struct{}{}
			defer func() { <-sem }()

			// Check context cancellation
			select {
			case <-ctx.Done():
				results <- &batchResult{batchIndex: idx, err: ctx.Err()}
				return
			default:
			}

			// Send progress update with progress percentage
			progress := float64(idx) / float64(len(batches))
			c.sendProgress(tctx, ProgressUpdate{
				Status:       StatusProcessingBatch,
				Message:      fmt.Sprintf("Processing batch %d of %d", idx+1, len(batches)),
				Detail:       fmt.Sprintf("%d images in this batch", len(imgs)),
				CurrentBatch: idx + 1,
				TotalBatches: len(batches),
				Progress:     progress,
				Model:        model,
			})

			if c.debug {
				fmt.Printf("[DEBUG] Processing batch %d/%d (%d images)...\n", idx+1, len(batches), len(imgs))
			}

			pages, tokens, err := c.processBatchWithProgress(ctx, imgs, req, model, tctx, idx+1)
			results <- &batchResult{
				batchIndex: idx,
				pages:      pages,
				tokens:     tokens,
				err:        err,
			}
		}(i, batch)
	}

	// Close results channel when all workers are done
	go func() {
		wg.Wait()
		close(results)
	}()

	// Collect results
	resultMap := make(map[int]*batchResult)
	for result := range results {
		if result.err != nil {
			return nil, 0, fmt.Errorf("batch %d failed: %w", result.batchIndex+1, result.err)
		}
		resultMap[result.batchIndex] = result
	}

	// Combine results in order
	var allPages []*PageContent
	totalTokens := 0

	for i := 0; i < len(batches); i++ {
		result := resultMap[i]
		allPages = append(allPages, result.pages...)
		totalTokens += result.tokens
	}

	return allPages, totalTokens, nil
}

// processBatchesSequential processes batches one by one (for small jobs)
func (c *Client) processBatchesSequential(ctx context.Context, batches [][]*ImageInfo, req *TranscribeRequest, model string) ([]*PageContent, int, error) {
	return c.processBatchesSequentialWithProgress(ctx, batches, req, model, nil)
}

// processBatchesSequentialWithProgress processes batches sequentially with progress updates
func (c *Client) processBatchesSequentialWithProgress(ctx context.Context, batches [][]*ImageInfo, req *TranscribeRequest, model string, tctx *transcribeContext) ([]*PageContent, int, error) {
	var allPages []*PageContent
	totalTokens := 0
	totalBatches := len(batches)

	for i, batch := range batches {
		// Calculate progress percentage
		progress := float64(i) / float64(totalBatches)

		// Send progress update
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusProcessingBatch,
			Message:      fmt.Sprintf("Processing batch %d of %d", i+1, totalBatches),
			Detail:       fmt.Sprintf("%d images in this batch", len(batch)),
			CurrentBatch: i + 1,
			TotalBatches: totalBatches,
			Progress:     progress,
			Model:        model,
		})

		if c.debug {
			fmt.Printf("[DEBUG] Processing batch %d/%d (%d images)...\n", i+1, totalBatches, len(batch))
		}

		pages, tokens, err := c.processBatchWithProgress(ctx, batch, req, model, tctx, i+1)
		if err != nil {
			return nil, 0, fmt.Errorf("batch %d failed: %w", i+1, err)
		}

		allPages = append(allPages, pages...)
		totalTokens += tokens

		// Update tokens used and progress after batch complete
		if tctx != nil {
			tctx.tokensUsed = totalTokens
		}

		// Send completion progress for this batch
		batchProgress := float64(i+1) / float64(totalBatches)
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusParsingResponse,
			Message:      fmt.Sprintf("Batch %d complete", i+1),
			Detail:       fmt.Sprintf("%d pages extracted", len(pages)),
			CurrentBatch: i + 1,
			TotalBatches: totalBatches,
			Progress:     batchProgress,
			Model:        model,
			ResponseInfo: &AIResponseInfo{
				TokensTotal:    tokens,
				ItemsProcessed: len(pages),
			},
		})
	}

	return allPages, totalTokens, nil
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
	return c.processBatchWithProgress(ctx, images, req, model, nil, 0)
}

// processBatchWithProgress processes a batch with progress updates
func (c *Client) processBatchWithProgress(ctx context.Context, images []*ImageInfo, req *TranscribeRequest, model string, tctx *transcribeContext, batchNum int) ([]*PageContent, int, error) {
	// Calculate total data size for transparency
	var totalDataSize int64
	for _, img := range images {
		totalDataSize += img.Size
	}

	// Build endpoint URL for transparency
	var endpoint string
	switch c.provider {
	case ProviderLocal:
		endpoint = c.baseURL + "/v1/chat/completions"
	case ProviderAzureAnthropic:
		endpoint = c.baseURL + "/v1/messages"
	default:
		endpoint = fmt.Sprintf("https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent", model)
	}

	// Build prompt preview for transparency
	promptPreview := "Extract text from images, output JSON with page contents..."

	// Send progress: sending request with transparency info
	c.sendProgress(tctx, ProgressUpdate{
		Status:       StatusSendingRequest,
		Message:      "Sending images to AI",
		Detail:       fmt.Sprintf("Batch %d: %d images", batchNum, len(images)),
		CurrentBatch: batchNum,
		Model:        model,
		RequestInfo: &AIRequestInfo{
			Endpoint:      endpoint,
			Method:        "POST",
			ContentType:   "application/json",
			ImageCount:    len(images),
			TotalDataSize: totalDataSize,
			PromptPreview: promptPreview,
			Parameters: map[string]string{
				"model":             model,
				"max_output_tokens": "8192",
				"response_format":   "application/json",
			},
		},
	})

	var pages []*PageContent
	var tokens int
	var err error
	startTime := time.Now()

	// Route to appropriate provider
	switch c.provider {
	case ProviderLocal:
		// Send progress: waiting for response
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusWaitingResponse,
			Message:      "Waiting for Local LLM response",
			Detail:       fmt.Sprintf("Model: %s", c.model),
			CurrentBatch: batchNum,
			Model:        c.model,
		})
		pages, tokens, err = c.processBatchLocal(ctx, images, req)
	case ProviderAzureAnthropic:
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusWaitingResponse,
			Message:      "Waiting for Claude response",
			Detail:       fmt.Sprintf("Model: %s", c.model),
			CurrentBatch: batchNum,
			Model:        c.model,
		})
		pages, tokens, err = c.processBatchAzureAnthropic(ctx, images, req)
	default:
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusWaitingResponse,
			Message:      "Waiting for Gemini response",
			Detail:       fmt.Sprintf("Model: %s", model),
			CurrentBatch: batchNum,
			Model:        model,
		})
		pages, tokens, err = c.processBatchGemini(ctx, images, req, model)
	}

	latency := time.Since(startTime)

	if err != nil {
		// Send error with transparency info
		c.sendProgress(tctx, ProgressUpdate{
			Status:       StatusError,
			Message:      "Request failed",
			Detail:       err.Error(),
			CurrentBatch: batchNum,
			Model:        model,
			Error:        err,
			ResponseInfo: &AIResponseInfo{
				Latency:      latency,
				ErrorMessage: err.Error(),
			},
		})
		return nil, 0, err
	}

	// Send progress: parsing complete with transparency info
	c.sendProgress(tctx, ProgressUpdate{
		Status:       StatusParsingResponse,
		Message:      "Response received from " + c.getProviderDisplayName(),
		Detail:       fmt.Sprintf("Batch %d: %d pages extracted, %d tokens", batchNum, len(pages), tokens),
		CurrentBatch: batchNum,
		Model:        model,
		ResponseInfo: &AIResponseInfo{
			StatusCode:     200,
			StatusText:     "OK",
			Latency:        latency,
			TokensTotal:    tokens,
			ItemsProcessed: len(pages),
		},
	})

	return pages, tokens, nil
}

// processBatchGemini processes a batch using Gemini API
func (c *Client) processBatchGemini(ctx context.Context, images []*ImageInfo, req *TranscribeRequest, model string) ([]*PageContent, int, error) {
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
			MaxOutputTokens:  intPtr(8192),
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

// processBatchAzureAnthropic processes a batch using Azure Anthropic (Claude) API
func (c *Client) processBatchAzureAnthropic(ctx context.Context, images []*ImageInfo, req *TranscribeRequest) ([]*PageContent, int, error) {
	if c.anthropicClient == nil {
		return nil, 0, fmt.Errorf("Anthropic client not initialized")
	}

	// Build the prompt (use the same prompt as Gemini)
	prompt := c.buildExtractionPrompt(images, req)

	// Build content blocks with images
	contentBlocks := make([]anthropic.ContentBlockParamUnion, 0, len(images)+1)

	// Add images first
	for _, img := range images {
		data, err := os.ReadFile(img.Path)
		if err != nil {
			return nil, 0, fmt.Errorf("failed to read %s: %w", img.Filename, err)
		}

		// Use the helper function to create image block with base64 data
		contentBlocks = append(contentBlocks, anthropic.NewImageBlockBase64(
			img.MIMEType,
			base64.StdEncoding.EncodeToString(data),
		))
	}

	// Add prompt text
	contentBlocks = append(contentBlocks, anthropic.NewTextBlock(prompt))

	if c.debug {
		fmt.Printf("[DEBUG] Azure Anthropic request to model: %s with %d images\n", c.model, len(images))
	}

	// Create the message request
	message, err := c.anthropicClient.Messages.New(ctx, anthropic.MessageNewParams{
		Model:     anthropic.Model(c.model),
		MaxTokens: 8192,
		Messages: []anthropic.MessageParam{
			{
				Role:    anthropic.MessageParamRoleUser,
				Content: contentBlocks,
			},
		},
	})
	if err != nil {
		return nil, 0, fmt.Errorf("Azure Anthropic request failed: %w", err)
	}

	// Extract text content from response
	var textContent string
	for _, block := range message.Content {
		switch b := block.AsAny().(type) {
		case anthropic.TextBlock:
			textContent = b.Text
			break
		}
	}

	if textContent == "" {
		return nil, 0, fmt.Errorf("no content in Azure Anthropic response")
	}

	if c.debug {
		if len(textContent) < 500 {
			fmt.Printf("[DEBUG] Azure Anthropic response: %s\n", textContent)
		} else {
			fmt.Printf("[DEBUG] Azure Anthropic response (truncated): %s...\n", textContent[:500])
		}
	}

	// Parse the response (same format as Gemini)
	pageContents, err := c.parseAzureAnthropicResponse(textContent, images)
	if err != nil {
		return nil, 0, err
	}

	tokens := 0
	if message.Usage.InputTokens > 0 || message.Usage.OutputTokens > 0 {
		tokens = int(message.Usage.InputTokens + message.Usage.OutputTokens)
	}

	return pageContents, tokens, nil
}

// parseAzureAnthropicResponse parses the Azure Anthropic response into page contents
func (c *Client) parseAzureAnthropicResponse(text string, images []*ImageInfo) ([]*PageContent, error) {
	// Try to parse as JSON (same format as Gemini)
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

// processBatchLocal processes a batch using local LLM (LM Studio, Ollama, etc.)
func (c *Client) processBatchLocal(ctx context.Context, images []*ImageInfo, req *TranscribeRequest) ([]*PageContent, int, error) {
	// Build a simplified prompt for local LLM (shorter to save context)
	prompt := c.buildLocalLLMPrompt(images, req)

	// Build content array with images
	content := make([]LocalLLMContent, 0, len(images)+1)

	// Resize options for local LLM - smaller images to fit context
	resizeOpts := ResizeOptions{
		MaxWidth:  768, // Smaller for local LLM context limits
		MaxHeight: 768,
		Quality:   80,
	}

	// Add images first (as base64 data URLs)
	for _, img := range images {
		// Resize image to reduce token usage
		data, mimeType, err := ResizeImageIfNeeded(img.Path, 500*1024, resizeOpts) // 500KB threshold
		if err != nil {
			return nil, 0, fmt.Errorf("failed to process %s: %w", img.Filename, err)
		}

		if c.debug {
			origInfo, _ := os.Stat(img.Path)
			fmt.Printf("[DEBUG] Image %s: original %.2f KB -> resized %.2f KB\n",
				img.Filename, float64(origInfo.Size())/1024, float64(len(data))/1024)
		}

		// Format as data URL for local LLM vision models
		dataURL := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))

		content = append(content, LocalLLMContent{
			Type: "image_url",
			ImageURL: &LocalLLMImageURL{
				URL:    dataURL,
				Detail: "high",
			},
		})
	}

	// Add prompt text
	content = append(content, LocalLLMContent{
		Type: "text",
		Text: prompt,
	})

	// Build request
	apiReq := &LocalLLMRequest{
		Model: c.model,
		Messages: []LocalLLMMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		MaxTokens:   8192,
		Temperature: 0.1,
	}

	if req.Temperature != nil {
		apiReq.Temperature = *req.Temperature
	}

	// Make API call
	resp, err := c.generateContentLocal(ctx, apiReq)
	if err != nil {
		return nil, 0, err
	}

	// Parse response
	pageContents, err := c.parseLocalResponse(resp, images)
	if err != nil {
		return nil, 0, err
	}

	tokens := 0
	if resp.Usage != nil {
		tokens = resp.Usage.TotalTokens
	}

	// Stage 2: If text model is configured, refine the extracted text
	if c.textModel != "" {
		if c.debug {
			fmt.Printf("[DEBUG] Two-stage pipeline: refining with text model %s\n", c.textModel)
		}
		refinedContents, refinedTokens, err := c.refineWithTextModel(ctx, pageContents, req)
		if err != nil {
			// Log warning but don't fail - return vision model output
			if c.debug {
				fmt.Printf("[DEBUG] Text model refinement failed: %v (using vision model output)\n", err)
			}
		} else {
			pageContents = refinedContents
			tokens += refinedTokens
		}
	}

	return pageContents, tokens, nil
}

// refineWithTextModel sends extracted text to the text/agentic model for refinement
func (c *Client) refineWithTextModel(ctx context.Context, pages []*PageContent, req *TranscribeRequest) ([]*PageContent, int, error) {
	// Build the refinement prompt
	prompt := c.buildTextRefinementPrompt(pages, req)

	// Build request for text model (no images, just text)
	content := []LocalLLMContent{
		{
			Type: "text",
			Text: prompt,
		},
	}

	endpoint := c.textEndpoint
	if endpoint == "" {
		endpoint = c.baseURL
	}

	apiReq := &LocalLLMRequest{
		Model: c.textModel,
		Messages: []LocalLLMMessage{
			{
				Role:    "user",
				Content: content,
			},
		},
		MaxTokens:   16384, // More tokens for refined output
		Temperature: 0.2,   // Slightly more creative for better prose
	}

	if req.Temperature != nil {
		apiReq.Temperature = *req.Temperature
	}

	// Make API call to text model endpoint
	resp, err := c.generateContentLocalWithEndpoint(ctx, endpoint, apiReq)
	if err != nil {
		return nil, 0, fmt.Errorf("text model refinement failed: %w", err)
	}

	// Parse the refined response
	refinedPages, err := c.parseRefinementResponse(resp, pages)
	if err != nil {
		return nil, 0, err
	}

	tokens := 0
	if resp.Usage != nil {
		tokens = resp.Usage.TotalTokens
	}

	return refinedPages, tokens, nil
}

// buildTextRefinementPrompt creates a prompt for the text model to refine extracted content
func (c *Client) buildTextRefinementPrompt(pages []*PageContent, req *TranscribeRequest) string {
	var sb strings.Builder

	sb.WriteString(`You are an expert document editor and markdown specialist. You have been given raw text extracted from document images by a vision model. Your task is to refine this text into well-formatted, polished markdown.

## Your Tasks:
1. **Fix OCR errors**: Correct obvious typos and misrecognized characters
2. **Improve formatting**: Ensure proper markdown structure (headings, lists, tables, etc.)
3. **Maintain accuracy**: Do NOT add, remove, or change the meaning of any content
4. **Preserve structure**: Keep chapter/section organization intact
5. **Clean up artifacts**: Remove scanning artifacts, page numbers if redundant, etc.

## Output Format:
Return JSON with the refined pages:
{
  "pages": [
    {
      "page_number": 1,
      "text": "Refined markdown content",
      "has_heading": true,
      "heading_text": "Chapter Title",
      "heading_level": 1,
      "is_chapter_start": true,
      "chapter_title": "Chapter Title",
      "images": []
    }
  ]
}

`)

	if req.Language != "" {
		sb.WriteString(fmt.Sprintf("Document language: %s\n\n", req.Language))
	}

	if req.PreserveFormatting {
		sb.WriteString("IMPORTANT: Pay special attention to preserving tables, lists, and special formatting.\n\n")
	}

	sb.WriteString("## Raw Extracted Text to Refine:\n\n")

	// Include the raw extracted text from all pages
	for _, page := range pages {
		sb.WriteString(fmt.Sprintf("### Page %d:\n", page.PageNumber))
		sb.WriteString(page.Text)
		sb.WriteString("\n\n---\n\n")
	}

	sb.WriteString("\nNow refine the above text into clean, well-formatted markdown. Output JSON only.")

	return sb.String()
}

// parseRefinementResponse parses the text model's refinement response
func (c *Client) parseRefinementResponse(resp *LocalLLMResponse, originalPages []*PageContent) ([]*PageContent, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in refinement response")
	}

	text := resp.Choices[0].Message.Content
	text = strings.TrimSpace(text)

	// Clean up markdown code blocks
	if strings.HasPrefix(text, "```json") {
		text = strings.TrimPrefix(text, "```json")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	} else if strings.HasPrefix(text, "```") {
		text = strings.TrimPrefix(text, "```")
		text = strings.TrimSuffix(text, "```")
		text = strings.TrimSpace(text)
	}

	var result struct {
		Pages []*PageContent `json:"pages"`
	}

	if err := json.Unmarshal([]byte(text), &result); err != nil {
		if c.debug {
			fmt.Printf("[DEBUG] Refinement JSON parse failed: %v\n", err)
		}
		// If JSON parsing fails, try to use the text as refined content for all pages
		// This handles cases where the model just outputs markdown without JSON wrapper
		if len(originalPages) == 1 {
			return []*PageContent{
				{
					PageNumber: originalPages[0].PageNumber,
					Text:       text,
				},
			}, nil
		}
		return nil, fmt.Errorf("failed to parse refinement response: %w", err)
	}

	// Preserve page numbers from original
	for i, page := range result.Pages {
		if i < len(originalPages) {
			page.PageNumber = originalPages[i].PageNumber
		}
	}

	return result.Pages, nil
}

// generateContentLocalWithEndpoint makes an API call to a specific endpoint
func (c *Client) generateContentLocalWithEndpoint(ctx context.Context, endpoint string, req *LocalLLMRequest) (*LocalLLMResponse, error) {
	// Build URL
	apiURL := fmt.Sprintf("%s/v1/chat/completions", endpoint)

	// Serialize request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.debug {
		fmt.Printf("[DEBUG] POST %s (text model)\n", apiURL)
		fmt.Printf("[DEBUG] Model: %s\n", req.Model)
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
		return nil, fmt.Errorf("text model request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if c.debug {
		fmt.Printf("[DEBUG] Text model response status: %d\n", resp.StatusCode)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("text model API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result LocalLLMResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse text model response: %w", err)
	}

	return &result, nil
}

// generateContentLocal makes an API call to local LLM
func (c *Client) generateContentLocal(ctx context.Context, req *LocalLLMRequest) (*LocalLLMResponse, error) {
	// Build URL
	apiURL := fmt.Sprintf("%s/v1/chat/completions", c.baseURL)

	// Serialize request
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	if c.debug {
		fmt.Printf("[DEBUG] POST %s\n", apiURL)
		fmt.Printf("[DEBUG] Model: %s, Messages: %d\n", req.Model, len(req.Messages))
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
		return nil, fmt.Errorf("request failed (is the LLM server running?): %w", err)
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
		errStr := string(respBody)
		// Check for context length error and provide helpful guidance
		if strings.Contains(errStr, "context") && strings.Contains(errStr, "overflow") {
			return nil, fmt.Errorf("context length exceeded. Try:\n"+
				"  1. Load your model with a larger context length in LM Studio/Ollama\n"+
				"  2. Use a model with larger context support\n"+
				"  3. The image may be too detailed - try smaller/simpler images\n"+
				"Original error: %s", errStr)
		}
		return nil, fmt.Errorf("API error (status %d): %s", resp.StatusCode, errStr)
	}

	// Parse response
	var result LocalLLMResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if result.Error != nil {
		errMsg := result.Error.Message
		// Check for context length error
		if strings.Contains(errMsg, "context") || strings.Contains(errMsg, "token") {
			return nil, fmt.Errorf("context length exceeded. Try:\n"+
				"  1. Load your model with a larger context length in LM Studio/Ollama\n"+
				"  2. Use a model with larger context support (8k+ tokens recommended)\n"+
				"  3. The image may be too detailed - try smaller/simpler images\n"+
				"Original error: %s", errMsg)
		}
		return nil, fmt.Errorf("API error: %s - %s", result.Error.Code, errMsg)
	}

	return &result, nil
}

// parseLocalResponse parses the local LLM response into page contents
func (c *Client) parseLocalResponse(resp *LocalLLMResponse, images []*ImageInfo) ([]*PageContent, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	// Get the text content
	text := resp.Choices[0].Message.Content

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

// buildLocalLLMPrompt creates a shorter prompt optimized for local LLM context limits
func (c *Client) buildLocalLLMPrompt(images []*ImageInfo, req *TranscribeRequest) string {
	var sb strings.Builder

	sb.WriteString(`Extract text from this image page. Output JSON:
{"pages":[{"page_number":1,"text":"extracted markdown text","has_heading":false,"heading_text":"","is_chapter_start":false}]}

Rules:
- Use markdown formatting (# headings, **bold**, lists, tables)
- Preserve original text layout
- Output valid JSON only
`)

	if req.Language != "" {
		sb.WriteString(fmt.Sprintf("- Document language: %s\n", req.Language))
	}

	sb.WriteString(fmt.Sprintf("\nPage %d:\n", images[0].PageIndex+1))

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
	return `To use image transcription, you need a backend configured.

Option 1: Local LLM (FREE - no API key needed!)

  LM Studio (recommended for vision models):
    1. Download from https://lmstudio.ai
    2. Load a vision-capable model (e.g., LLaVA, Qwen-VL)
    3. Start the local server (default port 1234)
    4. Set: export LLM_ENDPOINT="http://localhost:1234"
           export LLM_MODEL="your-vision-model"

  Ollama:
    1. Install from https://ollama.ai
    2. Run: ollama run llava
    3. Set: export LLM_ENDPOINT="http://localhost:11434"
           export LLM_MODEL="llava"

Option 2: Azure Anthropic (Claude)

  1. Deploy Claude in Azure AI Studio
  2. Set: export AZURE_ANTHROPIC_ENDPOINT="https://your-resource.services.ai.azure.com"
         export AZURE_ANTHROPIC_API_KEY="your-api-key"
         export AZURE_ANTHROPIC_MODEL="claude-sonnet-4-20250514"  # Optional

Option 3: Google Gemini API

  1. Go to https://aistudio.google.com/apikey
  2. Sign in with your Google account
  3. Click "Create API key"
  4. Copy the API key
  5. Set: export GEMINI_API_KEY="your-api-key"

Or create a .env file with these values.`
}

// CheckConfig verifies that a transcription backend is configured
func CheckConfig() error {
	// Check for local LLM first
	if os.Getenv("LLM_ENDPOINT") != "" || os.Getenv("IMAGE_LLM_ENDPOINT") != "" {
		return nil // Local LLM configured, no API key needed
	}

	// Check Azure Anthropic config
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" {
		if os.Getenv("AZURE_ANTHROPIC_API_KEY") == "" {
			return fmt.Errorf("AZURE_ANTHROPIC_API_KEY not set")
		}
		return nil // Azure Anthropic configured
	}

	// Check Gemini config
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GOOGLE_API_KEY")
	}
	if apiKey == "" {
		return fmt.Errorf("no backend configured. Set LLM_ENDPOINT for local LLM, AZURE_ANTHROPIC_ENDPOINT for Azure Anthropic, or GEMINI_API_KEY for Gemini")
	}
	return nil
}

// GetProvider returns the current provider type based on environment
func GetProvider() Provider {
	if os.Getenv("LLM_ENDPOINT") != "" || os.Getenv("IMAGE_LLM_ENDPOINT") != "" {
		return ProviderLocal
	}
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" {
		return ProviderAzureAnthropic
	}
	return ProviderGemini
}

// GetTextModel returns the configured text model for two-stage pipeline (empty if not configured)
func GetTextModel() string {
	return os.Getenv("IMAGE_TEXT_MODEL")
}

// GetTextEndpoint returns the configured text model endpoint (or empty for same as vision)
func GetTextEndpoint() string {
	return os.Getenv("IMAGE_TEXT_ENDPOINT")
}

// IsTwoStagePipeline returns true if a separate text model is configured
func IsTwoStagePipeline() bool {
	return os.Getenv("IMAGE_TEXT_MODEL") != ""
}
