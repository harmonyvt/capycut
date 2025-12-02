package video

import (
	"testing"
	"time"
)

func TestParseTimestamp(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{
			name:     "seconds only",
			input:    "30",
			expected: 30 * time.Second,
			wantErr:  false,
		},
		{
			name:     "decimal seconds",
			input:    "30.5",
			expected: 30*time.Second + 500*time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "MM:SS format",
			input:    "03:30",
			expected: 3*time.Minute + 30*time.Second,
			wantErr:  false,
		},
		{
			name:     "HH:MM:SS format",
			input:    "01:30:45",
			expected: 1*time.Hour + 30*time.Minute + 45*time.Second,
			wantErr:  false,
		},
		{
			name:     "HH:MM:SS with decimal seconds",
			input:    "00:03:30.5",
			expected: 3*time.Minute + 30*time.Second + 500*time.Millisecond,
			wantErr:  false,
		},
		{
			name:     "zero time",
			input:    "00:00:00",
			expected: 0,
			wantErr:  false,
		},
		{
			name:     "with whitespace",
			input:    "  03:30  ",
			expected: 3*time.Minute + 30*time.Second,
			wantErr:  false,
		},
		{
			name:     "invalid format",
			input:    "invalid",
			expected: 0,
			wantErr:  true,
		},
		{
			name:     "too many colons",
			input:    "01:02:03:04",
			expected: 0,
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTimestamp(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Errorf("ParseTimestamp(%q) expected error, got nil", tt.input)
				}
				return
			}
			if err != nil {
				t.Errorf("ParseTimestamp(%q) unexpected error: %v", tt.input, err)
				return
			}
			if result != tt.expected {
				t.Errorf("ParseTimestamp(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    time.Duration
		expected string
	}{
		{
			name:     "zero duration",
			input:    0,
			expected: "00:00",
		},
		{
			name:     "seconds only",
			input:    45 * time.Second,
			expected: "00:45",
		},
		{
			name:     "minutes and seconds",
			input:    3*time.Minute + 30*time.Second,
			expected: "03:30",
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
			result := FormatDuration(tt.input)
			if result != tt.expected {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestCalculateClipDuration(t *testing.T) {
	tests := []struct {
		name      string
		startTime string
		endTime   string
		expected  time.Duration
		wantErr   bool
	}{
		{
			name:      "simple duration",
			startTime: "00:01:00",
			endTime:   "00:03:00",
			expected:  2 * time.Minute,
			wantErr:   false,
		},
		{
			name:      "MM:SS format",
			startTime: "03:00",
			endTime:   "05:30",
			expected:  2*time.Minute + 30*time.Second,
			wantErr:   false,
		},
		{
			name:      "with hours",
			startTime: "01:00:00",
			endTime:   "01:30:00",
			expected:  30 * time.Minute,
			wantErr:   false,
		},
		{
			name:      "invalid start time",
			startTime: "invalid",
			endTime:   "00:03:00",
			expected:  0,
			wantErr:   true,
		},
		{
			name:      "invalid end time",
			startTime: "00:01:00",
			endTime:   "invalid",
			expected:  0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateClipDuration(tt.startTime, tt.endTime)
			if tt.wantErr {
				if err == nil {
					t.Errorf("CalculateClipDuration(%q, %q) expected error, got nil", tt.startTime, tt.endTime)
				}
				return
			}
			if err != nil {
				t.Errorf("CalculateClipDuration(%q, %q) unexpected error: %v", tt.startTime, tt.endTime, err)
				return
			}
			if result != tt.expected {
				t.Errorf("CalculateClipDuration(%q, %q) = %v, want %v", tt.startTime, tt.endTime, result, tt.expected)
			}
		})
	}
}

func TestIsVideoFile(t *testing.T) {
	tests := []struct {
		name     string
		path     string
		expected bool
	}{
		{"mp4 file", "video.mp4", true},
		{"MP4 uppercase", "video.MP4", true},
		{"mkv file", "movie.mkv", true},
		{"mov file", "clip.mov", true},
		{"avi file", "old.avi", true},
		{"webm file", "web.webm", true},
		{"text file", "document.txt", false},
		{"image file", "photo.jpg", false},
		{"no extension", "video", false},
		{"hidden video", ".video.mp4", true},
		{"path with directory", "/path/to/video.mp4", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsVideoFile(tt.path)
			if result != tt.expected {
				t.Errorf("IsVideoFile(%q) = %v, want %v", tt.path, result, tt.expected)
			}
		})
	}
}

func TestGenerateOutputPath(t *testing.T) {
	tests := []struct {
		name      string
		inputPath string
		startTime string
		endTime   string
		contains  []string // substrings the output should contain
	}{
		{
			name:      "basic mp4",
			inputPath: "/videos/test.mp4",
			startTime: "00:01:00",
			endTime:   "00:02:00",
			contains:  []string{"test", "clip", ".mp4", "00-01-00", "00-02-00"},
		},
		{
			name:      "mkv file",
			inputPath: "/path/movie.mkv",
			startTime: "01:30:00",
			endTime:   "02:00:00",
			contains:  []string{"movie", "clip", ".mkv"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := GenerateOutputPath(tt.inputPath, tt.startTime, tt.endTime)
			for _, substr := range tt.contains {
				if !containsString(result, substr) {
					t.Errorf("GenerateOutputPath() = %q, expected to contain %q", result, substr)
				}
			}
		})
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
