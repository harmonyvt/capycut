package gemini

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewClient(t *testing.T) {
	tests := []struct {
		name    string
		apiKey  string
		wantErr bool
	}{
		{"valid key", "test-api-key", false},
		{"empty key", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewClient(tt.apiKey)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewClient() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && client == nil {
				t.Error("NewClient() returned nil client")
			}
		})
	}
}

func TestNewClientFromEnv(t *testing.T) {
	// Save original env
	origGemini := os.Getenv("GEMINI_API_KEY")
	origGoogle := os.Getenv("GOOGLE_API_KEY")
	defer func() {
		os.Setenv("GEMINI_API_KEY", origGemini)
		os.Setenv("GOOGLE_API_KEY", origGoogle)
	}()

	// Test with GEMINI_API_KEY
	os.Setenv("GEMINI_API_KEY", "test-gemini-key")
	os.Unsetenv("GOOGLE_API_KEY")
	client, err := NewClientFromEnv()
	if err != nil {
		t.Errorf("NewClientFromEnv() with GEMINI_API_KEY failed: %v", err)
	}
	if client == nil {
		t.Error("NewClientFromEnv() returned nil client")
	}

	// Test with GOOGLE_API_KEY fallback
	os.Unsetenv("GEMINI_API_KEY")
	os.Setenv("GOOGLE_API_KEY", "test-google-key")
	client, err = NewClientFromEnv()
	if err != nil {
		t.Errorf("NewClientFromEnv() with GOOGLE_API_KEY failed: %v", err)
	}
	if client == nil {
		t.Error("NewClientFromEnv() returned nil client")
	}

	// Test with no keys
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	_, err = NewClientFromEnv()
	if err == nil {
		t.Error("NewClientFromEnv() should fail with no API keys set")
	}
}

func TestClientOptions(t *testing.T) {
	client, err := NewClient("test-key",
		WithBaseURL("https://custom.api.com"),
		WithDebug(true),
	)
	if err != nil {
		t.Fatalf("NewClient() failed: %v", err)
	}

	if client.baseURL != "https://custom.api.com" {
		t.Errorf("WithBaseURL() = %v, want https://custom.api.com", client.baseURL)
	}
	if !client.debug {
		t.Error("WithDebug(true) did not enable debug mode")
	}
}

func TestWithBaseURL_Invalid(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantURL string
	}{
		{"empty", "", BaseURL},
		{"invalid scheme", "ftp://example.com", BaseURL},
		{"no host", "http://", BaseURL},
		{"valid http", "http://localhost:8080", "http://localhost:8080"},
		{"valid https", "https://api.example.com", "https://api.example.com"},
		{"trailing slash", "https://api.example.com/", "https://api.example.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, _ := NewClient("test-key", WithBaseURL(tt.url))
			if client.baseURL != tt.wantURL {
				t.Errorf("WithBaseURL(%q) = %v, want %v", tt.url, client.baseURL, tt.wantURL)
			}
		})
	}
}

func TestGetMIMEType(t *testing.T) {
	tests := []struct {
		ext  string
		want string
	}{
		{".jpg", "image/jpeg"},
		{".jpeg", "image/jpeg"},
		{".png", "image/png"},
		{".gif", "image/gif"},
		{".webp", "image/webp"},
		{".bmp", "image/bmp"},
		{".tiff", "image/tiff"},
		{".tif", "image/tiff"},
		{".pdf", ""},
		{".txt", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			if got := getMIMEType(tt.ext); got != tt.want {
				t.Errorf("getMIMEType(%q) = %v, want %v", tt.ext, got, tt.want)
			}
		})
	}
}

func TestSanitizeFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Chapter 1: Introduction", "chapter_1_introduction"},
		{"Hello/World", "hello_world"},
		{"Test\\File", "test_file"},
		{"File:Name*With?Special<Chars>", "file_name_with_special_chars"},
		{"Multiple   Spaces", "multiple_spaces"},
		{"___Leading_Trailing___", "leading_trailing"},
		{"", "document"},
		{"A Very Long Title That Should Be Truncated Because It Exceeds Fifty Characters Limit", "a_very_long_title_that_should_be_truncated_because"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := sanitizeFilename(tt.input); got != tt.want {
				t.Errorf("sanitizeFilename(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestNaturalSort(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"page_1.png", "page_2.png", true},
		{"page_2.png", "page_10.png", true},
		{"page_10.png", "page_2.png", false},
		{"a.png", "b.png", true},
		{"img1.jpg", "img1.jpg", false},
		{"file_001.png", "file_002.png", true},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_"+tt.b, func(t *testing.T) {
			if got := naturalSort(tt.a, tt.b); got != tt.want {
				t.Errorf("naturalSort(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestIsImageFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"test.jpg", true},
		{"test.jpeg", true},
		{"test.png", true},
		{"test.gif", true},
		{"test.webp", true},
		{"test.bmp", true},
		{"test.tiff", true},
		{"test.tif", true},
		{"test.pdf", false},
		{"test.txt", false},
		{"test.doc", false},
		{"test.JPG", true}, // Case insensitive
		{"test.PNG", true},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isImageFile(tt.path); got != tt.want {
				t.Errorf("isImageFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestLoadImages(t *testing.T) {
	// Create temp directory with test images
	tmpDir := t.TempDir()

	// Create test image files (empty but valid names)
	testFiles := []string{"page_001.png", "page_002.png", "page_010.png", "page_003.png"}
	for _, name := range testFiles {
		path := filepath.Join(tmpDir, name)
		if err := os.WriteFile(path, []byte("fake image data"), 0644); err != nil {
			t.Fatalf("Failed to create test file: %v", err)
		}
	}

	// Test loading from directory
	images, err := LoadImages([]string{tmpDir})
	if err != nil {
		t.Fatalf("LoadImages() failed: %v", err)
	}

	if len(images) != 4 {
		t.Errorf("LoadImages() returned %d images, want 4", len(images))
	}

	// Check natural sort order
	expected := []string{"page_001.png", "page_002.png", "page_003.png", "page_010.png"}
	for i, img := range images {
		if filepath.Base(img) != expected[i] {
			t.Errorf("LoadImages()[%d] = %v, want %v", i, filepath.Base(img), expected[i])
		}
	}
}

func TestValidateImages(t *testing.T) {
	tmpDir := t.TempDir()

	// Create valid image
	validPath := filepath.Join(tmpDir, "valid.png")
	if err := os.WriteFile(validPath, []byte("image data"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Test valid image
	if err := ValidateImages([]string{validPath}); err != nil {
		t.Errorf("ValidateImages() failed for valid image: %v", err)
	}

	// Test non-existent file
	if err := ValidateImages([]string{"/nonexistent/file.png"}); err == nil {
		t.Error("ValidateImages() should fail for non-existent file")
	}

	// Test invalid extension
	invalidPath := filepath.Join(tmpDir, "invalid.txt")
	if err := os.WriteFile(invalidPath, []byte("text"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	if err := ValidateImages([]string{invalidPath}); err == nil {
		t.Error("ValidateImages() should fail for invalid extension")
	}
}

func TestAPIError(t *testing.T) {
	err := &APIError{
		StatusCode: 400,
		Message:    "Bad Request",
		Details:    "Invalid parameter",
	}

	expected := "Bad Request: Invalid parameter"
	if err.Error() != expected {
		t.Errorf("APIError.Error() = %v, want %v", err.Error(), expected)
	}

	err2 := &APIError{
		StatusCode: 401,
		Message:    "Unauthorized",
	}
	if err2.Error() != "Unauthorized" {
		t.Errorf("APIError.Error() = %v, want Unauthorized", err2.Error())
	}
}

func TestTranscribeRequest_Validation(t *testing.T) {
	client, _ := NewClient("test-key")

	// Test empty images
	_, err := client.TranscribeImages(context.Background(), &TranscribeRequest{
		Images: []string{},
	})
	if err == nil {
		t.Error("TranscribeImages() should fail with empty images")
	}

	// Test too many images
	manyImages := make([]string, MaxTotalImages+1)
	for i := range manyImages {
		manyImages[i] = "/fake/image.png"
	}
	_, err = client.TranscribeImages(context.Background(), &TranscribeRequest{
		Images: manyImages,
	})
	if err == nil {
		t.Error("TranscribeImages() should fail with too many images")
	}
}

func TestGenerateContent_MockServer(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request
		if r.Method != "POST" {
			t.Errorf("Expected POST, got %s", r.Method)
		}

		// Return mock response
		resp := GenerateContentResponse{
			Candidates: []*Candidate{
				{
					Content: &Content{
						Parts: []*Part{
							{Text: `{"pages": [{"page_number": 1, "text": "Test content", "has_heading": false}]}`},
						},
					},
					FinishReason: "STOP",
				},
			},
			UsageMetadata: &UsageMetadata{
				PromptTokenCount:     100,
				CandidatesTokenCount: 50,
				TotalTokenCount:      150,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client, _ := NewClient("test-key", WithBaseURL(server.URL))

	// Create test image
	tmpDir := t.TempDir()
	imgPath := filepath.Join(tmpDir, "test.png")
	if err := os.WriteFile(imgPath, []byte("fake png data"), 0644); err != nil {
		t.Fatalf("Failed to create test image: %v", err)
	}

	// Make request
	resp, err := client.TranscribeImages(context.Background(), &TranscribeRequest{
		Images: []string{imgPath},
	})

	if err != nil {
		t.Fatalf("TranscribeImages() failed: %v", err)
	}

	if resp.TokensUsed != 150 {
		t.Errorf("TokensUsed = %d, want 150", resp.TokensUsed)
	}

	if len(resp.Documents) == 0 {
		t.Error("Expected at least one document")
	}
}

func TestWriteDocuments(t *testing.T) {
	tmpDir := t.TempDir()

	docs := []*MarkdownDocument{
		{
			Filename: "chapter_1.md",
			Title:    "Chapter 1",
			Content:  "# Chapter 1\n\nThis is chapter 1.",
			PageRange: PageRange{Start: 1, End: 5},
		},
		{
			Filename: "chapter_2.md",
			Title:    "Chapter 2",
			Content:  "# Chapter 2\n\nThis is chapter 2.",
			PageRange: PageRange{Start: 6, End: 10},
		},
	}

	result, err := WriteDocuments(docs, WriteOptions{
		OutputDir:       tmpDir,
		CreateIndexFile: true,
		AddFrontMatter:  true,
	})

	if err != nil {
		t.Fatalf("WriteDocuments() failed: %v", err)
	}

	// Should have 3 files: 2 chapters + index
	if len(result.FilesWritten) != 3 {
		t.Errorf("FilesWritten = %d, want 3", len(result.FilesWritten))
	}

	// Verify files exist
	for _, path := range result.FilesWritten {
		if _, err := os.Stat(path); err != nil {
			t.Errorf("File %s was not created: %v", path, err)
		}
	}

	// Verify front matter was added
	content, err := os.ReadFile(filepath.Join(tmpDir, "chapter_1.md"))
	if err != nil {
		t.Fatalf("Failed to read chapter_1.md: %v", err)
	}
	if len(content) == 0 {
		t.Error("chapter_1.md is empty")
	}
	contentStr := string(content)
	if !contains(contentStr, "---") {
		t.Error("Front matter not found in chapter_1.md")
	}
}

func TestWriteDocuments_NoOverwrite(t *testing.T) {
	tmpDir := t.TempDir()

	// Create existing file
	existingPath := filepath.Join(tmpDir, "existing.md")
	if err := os.WriteFile(existingPath, []byte("existing content"), 0644); err != nil {
		t.Fatalf("Failed to create existing file: %v", err)
	}

	docs := []*MarkdownDocument{
		{
			Filename: "existing.md",
			Title:    "Test",
			Content:  "new content",
		},
	}

	result, err := WriteDocuments(docs, WriteOptions{
		OutputDir: tmpDir,
		Overwrite: false,
	})

	if err != nil {
		t.Fatalf("WriteDocuments() failed: %v", err)
	}

	// Should have error about existing file
	if len(result.Errors) == 0 {
		t.Error("Expected error about existing file")
	}

	// Original file should be unchanged
	content, _ := os.ReadFile(existingPath)
	if string(content) != "existing content" {
		t.Error("Existing file was overwritten")
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		bytes int64
		want  string
	}{
		{0, "0 bytes"},
		{100, "100 bytes"},
		{1024, "1.00 KB"},
		{1536, "1.50 KB"},
		{1048576, "1.00 MB"},
		{1572864, "1.50 MB"},
		{1073741824, "1.00 GB"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := FormatSize(tt.bytes); got != tt.want {
				t.Errorf("FormatSize(%d) = %v, want %v", tt.bytes, got, tt.want)
			}
		})
	}
}

func TestCheckConfig(t *testing.T) {
	// Save original env
	origGemini := os.Getenv("GEMINI_API_KEY")
	origGoogle := os.Getenv("GOOGLE_API_KEY")
	defer func() {
		os.Setenv("GEMINI_API_KEY", origGemini)
		os.Setenv("GOOGLE_API_KEY", origGoogle)
	}()

	// Test with key set
	os.Setenv("GEMINI_API_KEY", "test-key")
	if err := CheckConfig(); err != nil {
		t.Errorf("CheckConfig() failed with key set: %v", err)
	}

	// Test with no keys
	os.Unsetenv("GEMINI_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	if err := CheckConfig(); err == nil {
		t.Error("CheckConfig() should fail with no keys set")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestCreateSmartBatches(t *testing.T) {
	client, _ := NewClient("test-key")

	tests := []struct {
		name       string
		imageSizes []int64 // sizes in bytes
		wantBatches int
	}{
		{
			name:       "small images fit in one batch",
			imageSizes: []int64{100000, 200000, 300000}, // ~600KB total
			wantBatches: 1,
		},
		{
			name:       "images split by size limit",
			imageSizes: []int64{5000000, 5000000, 5000000, 5000000}, // 4x5MB = 20MB
			wantBatches: 2, // Should split due to payload size limit
		},
		{
			name:       "many small images split by count",
			imageSizes: make([]int64, 25), // 25 small images
			wantBatches: 2, // Should split at MaxImagesPerRequest (20)
		},
		{
			name:       "single large image gets own batch",
			imageSizes: []int64{100000, 12000000, 100000}, // 100KB, 12MB, 100KB
			wantBatches: 3, // Large image needs its own batch
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock ImageInfo slice
			images := make([]*ImageInfo, len(tt.imageSizes))
			for i, size := range tt.imageSizes {
				if size == 0 {
					size = 10000 // Default small size for count test
				}
				images[i] = &ImageInfo{
					Path:      fmt.Sprintf("/fake/image_%d.png", i),
					Filename:  fmt.Sprintf("image_%d.png", i),
					Size:      size,
					PageIndex: i,
				}
			}

			batches := client.createSmartBatches(images)

			if len(batches) != tt.wantBatches {
				t.Errorf("createSmartBatches() created %d batches, want %d", len(batches), tt.wantBatches)
				for i, batch := range batches {
					totalSize := int64(0)
					for _, img := range batch {
						totalSize += img.Size
					}
					t.Logf("  Batch %d: %d images, %d bytes", i, len(batch), totalSize)
				}
			}

			// Verify all images are accounted for
			totalImages := 0
			for _, batch := range batches {
				totalImages += len(batch)
			}
			if totalImages != len(images) {
				t.Errorf("Total images in batches = %d, want %d", totalImages, len(images))
			}
		})
	}
}

func TestCreateSmartBatches_PayloadSizeLimit(t *testing.T) {
	client, _ := NewClient("test-key")

	// Create images that together exceed MaxPayloadSize
	// MaxPayloadSize is 15MB, with 1.4x overhead factor
	// So ~10.7MB of raw images should trigger a split
	images := []*ImageInfo{
		{Path: "/fake/1.png", Filename: "1.png", Size: 4000000, PageIndex: 0}, // 4MB
		{Path: "/fake/2.png", Filename: "2.png", Size: 4000000, PageIndex: 1}, // 4MB
		{Path: "/fake/3.png", Filename: "3.png", Size: 4000000, PageIndex: 2}, // 4MB -> total ~12MB raw, ~17MB with overhead
		{Path: "/fake/4.png", Filename: "4.png", Size: 1000000, PageIndex: 3}, // 1MB
	}

	batches := client.createSmartBatches(images)

	// Should create at least 2 batches due to size
	if len(batches) < 2 {
		t.Errorf("Expected at least 2 batches for large payload, got %d", len(batches))
	}

	// First batch should not exceed estimated payload limit
	const overheadFactor = 1.4
	for i, batch := range batches {
		totalEstimated := int64(0)
		for _, img := range batch {
			totalEstimated += int64(float64(img.Size) * overheadFactor)
		}
		if totalEstimated > MaxPayloadSize && len(batch) > 1 {
			t.Errorf("Batch %d estimated size %d exceeds MaxPayloadSize %d", i, totalEstimated, MaxPayloadSize)
		}
	}
}

func TestCreateSmartBatches_PreservesOrder(t *testing.T) {
	client, _ := NewClient("test-key")

	// Create 25 images
	images := make([]*ImageInfo, 25)
	for i := 0; i < 25; i++ {
		images[i] = &ImageInfo{
			Path:      fmt.Sprintf("/fake/page_%02d.png", i+1),
			Filename:  fmt.Sprintf("page_%02d.png", i+1),
			Size:      100000,
			PageIndex: i,
		}
	}

	batches := client.createSmartBatches(images)

	// Verify page order is preserved across batches
	expectedIndex := 0
	for _, batch := range batches {
		for _, img := range batch {
			if img.PageIndex != expectedIndex {
				t.Errorf("Page order broken: expected index %d, got %d", expectedIndex, img.PageIndex)
			}
			expectedIndex++
		}
	}
}
