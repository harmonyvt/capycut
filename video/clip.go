package video

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// ClipParams holds the parameters for clipping a video
type ClipParams struct {
	InputPath  string
	StartTime  string // Format: HH:MM:SS or MM:SS or seconds
	EndTime    string // Format: HH:MM:SS or MM:SS or seconds
	OutputPath string
}

// VideoInfo holds metadata about a video file
type VideoInfo struct {
	Duration time.Duration
	Path     string
	Filename string
}

// GetVideoInfo retrieves information about a video file using ffprobe
func GetVideoInfo(path string) (*VideoInfo, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get video info: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	durationSec, err := strconv.ParseFloat(durationStr, 64)
	if err != nil {
		return nil, fmt.Errorf("failed to parse duration: %w", err)
	}

	return &VideoInfo{
		Duration: time.Duration(durationSec * float64(time.Second)),
		Path:     path,
		Filename: filepath.Base(path),
	}, nil
}

// FormatDuration formats a duration as HH:MM:SS
func FormatDuration(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	if h > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}

// GenerateOutputPath creates an output path for the clipped video
func GenerateOutputPath(inputPath, startTime, endTime string) string {
	ext := filepath.Ext(inputPath)
	base := strings.TrimSuffix(filepath.Base(inputPath), ext)
	dir := filepath.Dir(inputPath)

	// Clean up time strings for filename
	startClean := strings.ReplaceAll(startTime, ":", "-")
	endClean := strings.ReplaceAll(endTime, ":", "-")

	return filepath.Join(dir, fmt.Sprintf("%s_clip_%s_to_%s%s", base, startClean, endClean, ext))
}

// ClipVideo clips a video using ffmpeg
func ClipVideo(params ClipParams) error {
	// Build ffmpeg command
	// Using -ss before -i for fast seeking, then -to for end time
	args := []string{
		"-y", // Overwrite output file if it exists
		"-ss", params.StartTime,
		"-i", params.InputPath,
		"-to", params.EndTime,
		"-c", "copy", // Copy streams without re-encoding (fast!)
		params.OutputPath,
	}

	cmd := exec.Command("ffmpeg", args...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffmpeg error: %w\nOutput: %s", err, string(output))
	}

	return nil
}

// CheckFFmpeg checks if ffmpeg is installed
func CheckFFmpeg() error {
	cmd := exec.Command("ffmpeg", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg not found. Please install ffmpeg first")
	}
	return nil
}

// CheckFFprobe checks if ffprobe is installed
func CheckFFprobe() error {
	cmd := exec.Command("ffprobe", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffprobe not found. Please install ffmpeg first")
	}
	return nil
}

// IsVideoFile checks if a file has a video extension
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	videoExts := map[string]bool{
		".mp4":  true,
		".mkv":  true,
		".mov":  true,
		".avi":  true,
		".webm": true,
		".flv":  true,
		".wmv":  true,
		".m4v":  true,
		".mpeg": true,
		".mpg":  true,
	}
	return videoExts[ext]
}

// CalculateClipDuration calculates the duration between two timestamps
func CalculateClipDuration(startTime, endTime string) (time.Duration, error) {
	start, err := ParseTimestamp(startTime)
	if err != nil {
		return 0, err
	}
	end, err := ParseTimestamp(endTime)
	if err != nil {
		return 0, err
	}
	return end - start, nil
}

// ParseTimestamp parses a timestamp string into a duration
// Supports formats: HH:MM:SS, MM:SS, SS, or decimal seconds
func ParseTimestamp(ts string) (time.Duration, error) {
	ts = strings.TrimSpace(ts)

	// Try parsing as decimal seconds first
	if secs, err := strconv.ParseFloat(ts, 64); err == nil {
		return time.Duration(secs * float64(time.Second)), nil
	}

	parts := strings.Split(ts, ":")
	switch len(parts) {
	case 1:
		// Just seconds
		secs, err := strconv.ParseFloat(parts[0], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid timestamp: %s", ts)
		}
		return time.Duration(secs * float64(time.Second)), nil
	case 2:
		// MM:SS
		mins, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes: %s", parts[0])
		}
		secs, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds: %s", parts[1])
		}
		return time.Duration(mins)*time.Minute + time.Duration(secs*float64(time.Second)), nil
	case 3:
		// HH:MM:SS
		hours, err := strconv.Atoi(parts[0])
		if err != nil {
			return 0, fmt.Errorf("invalid hours: %s", parts[0])
		}
		mins, err := strconv.Atoi(parts[1])
		if err != nil {
			return 0, fmt.Errorf("invalid minutes: %s", parts[1])
		}
		secs, err := strconv.ParseFloat(parts[2], 64)
		if err != nil {
			return 0, fmt.Errorf("invalid seconds: %s", parts[2])
		}
		return time.Duration(hours)*time.Hour + time.Duration(mins)*time.Minute + time.Duration(secs*float64(time.Second)), nil
	default:
		return 0, fmt.Errorf("invalid timestamp format: %s", ts)
	}
}
