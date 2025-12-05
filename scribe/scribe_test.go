package scribe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestTranscribeRequest tests request parameter validation
func TestTranscribeRequest(t *testing.T) {
	t.Run("empty request", func(t *testing.T) {
		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{})
		if err == nil || !strings.Contains(err.Error(), "either FilePath or CloudStorageURL must be specified") {
			t.Errorf("expected error for empty request, got: %v", err)
		}
	})

	// For parameter validation tests, we need to create a temp file first
	// because file existence is checked before parameter validation
	t.Run("invalid num_speakers too low", func(t *testing.T) {
		tmpFile, _ := os.CreateTemp("", "test*.mp3")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("test")
		tmpFile.Close()

		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{
			FilePath:    tmpFile.Name(),
			NumSpeakers: intPtr(0),
		})
		if err == nil || !strings.Contains(err.Error(), "num_speakers must be between 1 and 32") {
			t.Errorf("expected num_speakers validation error, got: %v", err)
		}
	})

	t.Run("invalid num_speakers too high", func(t *testing.T) {
		tmpFile, _ := os.CreateTemp("", "test*.mp3")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("test")
		tmpFile.Close()

		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{
			FilePath:    tmpFile.Name(),
			NumSpeakers: intPtr(33),
		})
		if err == nil || !strings.Contains(err.Error(), "num_speakers must be between 1 and 32") {
			t.Errorf("expected num_speakers validation error, got: %v", err)
		}
	})

	t.Run("invalid diarization_threshold too low", func(t *testing.T) {
		tmpFile, _ := os.CreateTemp("", "test*.mp3")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("test")
		tmpFile.Close()

		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{
			FilePath:             tmpFile.Name(),
			DiarizationThreshold: floatPtr(0.05),
		})
		if err == nil || !strings.Contains(err.Error(), "diarization_threshold must be between 0.1 and 0.4") {
			t.Errorf("expected diarization_threshold validation error, got: %v", err)
		}
	})

	t.Run("invalid diarization_threshold too high", func(t *testing.T) {
		tmpFile, _ := os.CreateTemp("", "test*.mp3")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("test")
		tmpFile.Close()

		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{
			FilePath:             tmpFile.Name(),
			DiarizationThreshold: floatPtr(0.5),
		})
		if err == nil || !strings.Contains(err.Error(), "diarization_threshold must be between 0.1 and 0.4") {
			t.Errorf("expected diarization_threshold validation error, got: %v", err)
		}
	})

	t.Run("invalid temperature", func(t *testing.T) {
		tmpFile, _ := os.CreateTemp("", "test*.mp3")
		defer os.Remove(tmpFile.Name())
		tmpFile.WriteString("test")
		tmpFile.Close()

		client, _ := NewClient("test-api-key")
		_, err := client.Transcribe(context.Background(), &TranscribeRequest{
			FilePath:    tmpFile.Name(),
			Temperature: floatPtr(3.0),
		})
		if err == nil || !strings.Contains(err.Error(), "temperature must be between 0.0 and 2.0") {
			t.Errorf("expected temperature validation error, got: %v", err)
		}
	})
}

// TestParseResponse tests transcript parsing
func TestParseResponse(t *testing.T) {
	resp := &TranscribeResponse{
		LanguageCode:        "en",
		LanguageProbability: 0.98,
		Text:                "Hello world. How are you?",
		Words: []Word{
			{Text: "Hello", Type: WordTypeWord, Start: 0.0, End: 0.5, SpeakerID: "speaker_0"},
			{Text: " ", Type: WordTypeSpacing, Start: 0.5, End: 0.5},
			{Text: "world", Type: WordTypeWord, Start: 0.5, End: 1.0, SpeakerID: "speaker_0"},
			{Text: ".", Type: WordTypeWord, Start: 1.0, End: 1.1, SpeakerID: "speaker_0"},
			{Text: " ", Type: WordTypeSpacing, Start: 1.1, End: 1.1},
			{Text: "laughter", Type: WordTypeAudioEvent, Start: 1.5, End: 2.0},
			{Text: "How", Type: WordTypeWord, Start: 2.5, End: 2.8, SpeakerID: "speaker_1"},
			{Text: " ", Type: WordTypeSpacing, Start: 2.8, End: 2.8},
			{Text: "are", Type: WordTypeWord, Start: 2.8, End: 3.0, SpeakerID: "speaker_1"},
			{Text: " ", Type: WordTypeSpacing, Start: 3.0, End: 3.0},
			{Text: "you?", Type: WordTypeWord, Start: 3.0, End: 3.5, SpeakerID: "speaker_1"},
		},
	}

	transcript := ParseResponse(resp)

	// Test basic fields
	if transcript.Language != "en" {
		t.Errorf("expected language 'en', got %q", transcript.Language)
	}

	// Test speaker segments
	if len(transcript.Segments) != 2 {
		t.Errorf("expected 2 speaker segments, got %d", len(transcript.Segments))
	}

	if transcript.Segments[0].SpeakerID != "speaker_0" {
		t.Errorf("expected first segment speaker 'speaker_0', got %q", transcript.Segments[0].SpeakerID)
	}

	if transcript.Segments[1].SpeakerID != "speaker_1" {
		t.Errorf("expected second segment speaker 'speaker_1', got %q", transcript.Segments[1].SpeakerID)
	}

	// Test audio events
	if len(transcript.AudioEvents) != 1 {
		t.Errorf("expected 1 audio event, got %d", len(transcript.AudioEvents))
	}

	if transcript.AudioEvents[0].Description != "laughter" {
		t.Errorf("expected audio event 'laughter', got %q", transcript.AudioEvents[0].Description)
	}

	// Test speaker count
	if transcript.CountSpeakers() != 2 {
		t.Errorf("expected 2 speakers, got %d", transcript.CountSpeakers())
	}
}

// TestFormatAsScript tests script formatting
func TestFormatAsScript(t *testing.T) {
	transcript := &Transcript{
		Segments: []SpeakerSegment{
			{SpeakerID: "speaker_0", Text: "Hello world."},
			{SpeakerID: "speaker_1", Text: "How are you?"},
			{SpeakerID: "speaker_0", Text: "I'm fine."},
		},
	}

	script := transcript.FormatAsScript()

	if !strings.Contains(script, "Speaker 1:") {
		t.Error("expected script to contain 'Speaker 1:'")
	}
	if !strings.Contains(script, "Speaker 2:") {
		t.Error("expected script to contain 'Speaker 2:'")
	}
	if !strings.Contains(script, "Hello world.") {
		t.Error("expected script to contain 'Hello world.'")
	}
}

// TestSubtitleGeneration tests SRT and VTT generation
func TestSubtitleGeneration(t *testing.T) {
	transcript := &Transcript{
		Raw: &TranscribeResponse{
			Words: []Word{
				{Text: "Hello", Type: WordTypeWord, Start: 0.0, End: 0.5, SpeakerID: "speaker_0"},
				{Text: "world", Type: WordTypeWord, Start: 0.6, End: 1.0, SpeakerID: "speaker_0"},
				{Text: "laughter", Type: WordTypeAudioEvent, Start: 1.5, End: 2.0},
				{Text: "Goodbye", Type: WordTypeWord, Start: 2.5, End: 3.0, SpeakerID: "speaker_1"},
			},
		},
	}

	t.Run("SRT format", func(t *testing.T) {
		opts := DefaultSubtitleOptions()
		opts.Format = SubtitleFormatSRT
		opts.IncludeSpeakerLabels = false
		opts.IncludeAudioEvents = true

		entries, err := GenerateSubtitles(transcript, opts)
		if err != nil {
			t.Fatalf("failed to generate subtitles: %v", err)
		}

		srt := FormatSRT(entries, opts)

		if !strings.Contains(srt, "1\n00:00:00,000 -->") {
			t.Error("expected SRT to contain proper timestamp format")
		}
		if !strings.Contains(srt, "[laughter]") {
			t.Error("expected SRT to contain audio event")
		}
	})

	t.Run("VTT format", func(t *testing.T) {
		opts := DefaultSubtitleOptions()
		opts.Format = SubtitleFormatVTT
		opts.IncludeSpeakerLabels = true
		opts.IncludeAudioEvents = false

		entries, err := GenerateSubtitles(transcript, opts)
		if err != nil {
			t.Fatalf("failed to generate subtitles: %v", err)
		}

		vtt := FormatVTT(entries, opts)

		if !strings.HasPrefix(vtt, "WEBVTT") {
			t.Error("expected VTT to start with 'WEBVTT'")
		}
		if !strings.Contains(vtt, "00:00:00.000 -->") {
			t.Error("expected VTT to contain proper timestamp format")
		}
	})
}

// TestTimestampFormatting tests SRT and VTT timestamp formatting
func TestTimestampFormatting(t *testing.T) {
	tests := []struct {
		duration time.Duration
		wantSRT  string
		wantVTT  string
	}{
		{0, "00:00:00,000", "00:00:00.000"},
		{time.Second, "00:00:01,000", "00:00:01.000"},
		{time.Minute + 30*time.Second, "00:01:30,000", "00:01:30.000"},
		{time.Hour + 2*time.Minute + 3*time.Second + 456*time.Millisecond, "01:02:03,456", "01:02:03.456"},
	}

	for _, tt := range tests {
		t.Run(tt.wantSRT, func(t *testing.T) {
			gotSRT := formatSRTTimestamp(tt.duration)
			gotVTT := formatVTTTimestamp(tt.duration)

			if gotSRT != tt.wantSRT {
				t.Errorf("formatSRTTimestamp(%v) = %q, want %q", tt.duration, gotSRT, tt.wantSRT)
			}
			if gotVTT != tt.wantVTT {
				t.Errorf("formatVTTTimestamp(%v) = %q, want %q", tt.duration, gotVTT, tt.wantVTT)
			}
		})
	}
}

// TestIsVideoFile tests video file detection
func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"video.mp4", true},
		{"video.mkv", true},
		{"video.mov", true},
		{"video.avi", true},
		{"video.webm", true},
		{"audio.mp3", false},
		{"audio.wav", false},
		{"audio.m4a", false},
		{"document.pdf", false},
		{"image.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsVideoFile(tt.path)
			if got != tt.want {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestIsAudioFile tests audio file detection
func TestIsAudioFile(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"audio.mp3", true},
		{"audio.m4a", true},
		{"audio.wav", true},
		{"audio.flac", true},
		{"audio.ogg", true},
		{"video.mp4", false},
		{"document.pdf", false},
		{"image.png", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := IsAudioFile(tt.path)
			if got != tt.want {
				t.Errorf("IsAudioFile(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestGetSpeakerStats tests speaker statistics calculation
func TestGetSpeakerStats(t *testing.T) {
	transcript := &Transcript{
		Segments: []SpeakerSegment{
			{SpeakerID: "speaker_0", StartTime: 0, EndTime: 10 * time.Second},
			{SpeakerID: "speaker_1", StartTime: 10 * time.Second, EndTime: 15 * time.Second},
			{SpeakerID: "speaker_0", StartTime: 15 * time.Second, EndTime: 25 * time.Second},
		},
	}

	stats := transcript.GetSpeakerStats()

	if stats["speaker_0"] != 20*time.Second {
		t.Errorf("expected speaker_0 duration 20s, got %v", stats["speaker_0"])
	}
	if stats["speaker_1"] != 5*time.Second {
		t.Errorf("expected speaker_1 duration 5s, got %v", stats["speaker_1"])
	}

	dominant, duration := transcript.GetDominantSpeaker()
	if dominant != "speaker_0" {
		t.Errorf("expected dominant speaker 'speaker_0', got %q", dominant)
	}
	if duration != 20*time.Second {
		t.Errorf("expected dominant duration 20s, got %v", duration)
	}
}

// TestSearchText tests text search functionality
func TestSearchText(t *testing.T) {
	transcript := &Transcript{
		Segments: []SpeakerSegment{
			{SpeakerID: "speaker_0", Text: "Hello world"},
			{SpeakerID: "speaker_1", Text: "How are you"},
			{SpeakerID: "speaker_0", Text: "I'm doing well, world"},
		},
	}

	matches := transcript.SearchText("world")
	if len(matches) != 2 {
		t.Errorf("expected 2 matches for 'world', got %d", len(matches))
	}

	matches = transcript.SearchText("xyz")
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for 'xyz', got %d", len(matches))
	}

	// Test case insensitivity
	matches = transcript.SearchText("HELLO")
	if len(matches) != 1 {
		t.Errorf("expected 1 match for 'HELLO' (case insensitive), got %d", len(matches))
	}
}

// TestMockAPIServer tests the client against a mock server
func TestMockAPIServer(t *testing.T) {
	// Create a mock response
	mockResponse := &TranscribeResponse{
		LanguageCode:        "en",
		LanguageProbability: 0.99,
		Text:                "Hello world",
		Words: []Word{
			{Text: "Hello", Type: WordTypeWord, Start: 0.0, End: 0.5, SpeakerID: "speaker_0"},
			{Text: " ", Type: WordTypeSpacing, Start: 0.5, End: 0.5},
			{Text: "world", Type: WordTypeWord, Start: 0.5, End: 1.0, SpeakerID: "speaker_0"},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify request method and path
		if r.Method != "POST" {
			t.Errorf("expected POST request, got %s", r.Method)
		}
		if r.URL.Path != "/v1/speech-to-text" {
			t.Errorf("expected path /v1/speech-to-text, got %s", r.URL.Path)
		}

		// Verify API key header
		if r.Header.Get("xi-api-key") != "test-api-key" {
			t.Errorf("expected api key 'test-api-key', got %s", r.Header.Get("xi-api-key"))
		}

		// Verify content type is multipart
		contentType := r.Header.Get("Content-Type")
		if !strings.Contains(contentType, "multipart/form-data") {
			t.Errorf("expected multipart/form-data, got %s", contentType)
		}

		// Parse multipart form
		err := r.ParseMultipartForm(10 << 20) // 10MB max
		if err != nil {
			t.Errorf("failed to parse multipart form: %v", err)
		}

		// Verify model_id
		if r.FormValue("model_id") != "scribe_v1" {
			t.Errorf("expected model_id 'scribe_v1', got %s", r.FormValue("model_id"))
		}

		// Verify file was uploaded
		file, _, err := r.FormFile("file")
		if err != nil {
			t.Errorf("expected file in form: %v", err)
		} else {
			file.Close()
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(mockResponse)
	}))
	defer server.Close()

	// Create client
	client, err := NewClient("test-api-key", WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "test*.mp3")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write some data
	tmpFile.WriteString("fake audio data")
	tmpFile.Close()

	// Make request
	req := &TranscribeRequest{
		FilePath: tmpFile.Name(),
		Model:    ModelScribeV1,
	}

	resp, err := client.Transcribe(context.Background(), req)
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}

	if resp.Text != "Hello world" {
		t.Errorf("expected text 'Hello world', got %q", resp.Text)
	}
	if resp.LanguageCode != "en" {
		t.Errorf("expected language 'en', got %q", resp.LanguageCode)
	}
	if len(resp.Words) != 3 {
		t.Errorf("expected 3 words, got %d", len(resp.Words))
	}
}

// TestAPIError tests error handling
func TestAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(&APIError{
			Code:    "invalid_api_key",
			Message: "Invalid API key",
		})
	}))
	defer server.Close()

	client, _ := NewClient("invalid-key", WithBaseURL(server.URL))

	tmpFile, _ := os.CreateTemp("", "test*.mp3")
	defer os.Remove(tmpFile.Name())
	tmpFile.WriteString("fake audio")
	tmpFile.Close()

	_, err := client.Transcribe(context.Background(), &TranscribeRequest{
		FilePath: tmpFile.Name(),
	})

	if err == nil {
		t.Fatal("expected error for invalid API key")
	}

	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("expected APIError, got %T", err)
	}

	if apiErr.StatusCode != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", apiErr.StatusCode)
	}
	if apiErr.Code != "invalid_api_key" {
		t.Errorf("expected code 'invalid_api_key', got %q", apiErr.Code)
	}
}

// TestWriteSubtitleFile tests writing subtitle file to disk
func TestWriteSubtitleFile(t *testing.T) {
	entries := []SubtitleEntry{
		{Index: 1, StartTime: 0, EndTime: time.Second, Text: "Hello"},
		{Index: 2, StartTime: time.Second, EndTime: 2 * time.Second, Text: "World"},
	}

	tmpDir := t.TempDir()
	srtPath := filepath.Join(tmpDir, "test.srt")

	opts := DefaultSubtitleOptions()
	opts.IncludeSpeakerLabels = false

	err := WriteSubtitleFile(entries, srtPath, opts)
	if err != nil {
		t.Fatalf("failed to write subtitle file: %v", err)
	}

	content, err := os.ReadFile(srtPath)
	if err != nil {
		t.Fatalf("failed to read subtitle file: %v", err)
	}

	if !strings.Contains(string(content), "Hello") {
		t.Error("expected subtitle file to contain 'Hello'")
	}
	if !strings.Contains(string(content), "World") {
		t.Error("expected subtitle file to contain 'World'")
	}
}

// TestEstimateTranscriptionTime tests time estimation
func TestEstimateTranscriptionTime(t *testing.T) {
	tests := []struct {
		duration time.Duration
		minTime  time.Duration
		maxTime  time.Duration
	}{
		{time.Minute, 10 * time.Second, 20 * time.Second},
		{10 * time.Minute, 100 * time.Second, 120 * time.Second},
		{time.Hour, 600 * time.Second, 700 * time.Second},
	}

	for _, tt := range tests {
		t.Run(tt.duration.String(), func(t *testing.T) {
			est := EstimateTranscriptionTime(tt.duration)
			if est < tt.minTime || est > tt.maxTime {
				t.Errorf("EstimateTranscriptionTime(%v) = %v, want between %v and %v",
					tt.duration, est, tt.minTime, tt.maxTime)
			}
		})
	}
}

// TestClientCreation tests client creation
func TestClientCreation(t *testing.T) {
	t.Run("with API key", func(t *testing.T) {
		client, err := NewClient("test-key")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if client == nil {
			t.Error("expected non-nil client")
		}
	})

	t.Run("without API key", func(t *testing.T) {
		_, err := NewClient("")
		if err == nil {
			t.Error("expected error for empty API key")
		}
	})

	t.Run("with options", func(t *testing.T) {
		client, err := NewClient("test-key",
			WithBaseURL("https://custom.api.com"),
			WithTimeout(5*time.Minute),
			WithDebug(true),
		)
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
		if client.baseURL != "https://custom.api.com" {
			t.Errorf("expected custom base URL, got %s", client.baseURL)
		}
		if !client.debug {
			t.Error("expected debug to be true")
		}
	})
}

// TestSplitLongLines tests line splitting for subtitles
func TestSplitLongLines(t *testing.T) {
	entries := []SubtitleEntry{
		{
			Index:     1,
			StartTime: 0,
			EndTime:   5 * time.Second,
			Text:      "This is a very long line that should be split into multiple lines for better readability",
		},
	}

	result := SplitLongLines(entries, 40)

	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}

	lines := strings.Split(result[0].Text, "\n")
	for _, line := range lines {
		if len(line) > 40 {
			t.Errorf("line too long: %d chars, max 40", len(line))
		}
	}
}

// Helper functions
func intPtr(i int) *int {
	return &i
}

func floatPtr(f float64) *float64 {
	return &f
}

// Benchmark tests
func BenchmarkParseResponse(b *testing.B) {
	// Create a large response
	words := make([]Word, 1000)
	for i := range words {
		words[i] = Word{
			Text:      "word",
			Type:      WordTypeWord,
			Start:     float64(i) * 0.5,
			End:       float64(i)*0.5 + 0.4,
			SpeakerID: "speaker_0",
		}
	}

	resp := &TranscribeResponse{
		LanguageCode:        "en",
		LanguageProbability: 0.99,
		Text:                strings.Repeat("word ", 1000),
		Words:               words,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ParseResponse(resp)
	}
}

func BenchmarkGenerateSubtitles(b *testing.B) {
	words := make([]Word, 500)
	for i := range words {
		words[i] = Word{
			Text:      "word",
			Type:      WordTypeWord,
			Start:     float64(i) * 0.5,
			End:       float64(i)*0.5 + 0.4,
			SpeakerID: "speaker_0",
		}
	}

	transcript := &Transcript{
		Raw: &TranscribeResponse{
			Words: words,
		},
	}

	opts := DefaultSubtitleOptions()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		GenerateSubtitles(transcript, opts)
	}
}

// Integration test placeholder - requires actual API key
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}

	apiKey := os.Getenv("ELEVENLABS_API_KEY")
	if apiKey == "" {
		t.Skip("ELEVENLABS_API_KEY not set, skipping integration test")
	}

	// Integration tests would go here
	t.Log("Integration tests would run with a valid API key")
}

// Mock server for testing multipart uploads
func createMockServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, file io.Reader, filename string)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		err := r.ParseMultipartForm(32 << 20)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		file, header, err := r.FormFile("file")
		if err != nil {
			// File might be optional, that's ok
			handler(w, r, nil, "")
			return
		}
		defer file.Close()

		handler(w, r, file, header.Filename)
	}))
}
