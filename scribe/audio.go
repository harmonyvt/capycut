package scribe

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// MediaInfo contains metadata about an audio or video file
type MediaInfo struct {
	Path       string
	Duration   time.Duration
	SampleRate int
	Channels   int
	Codec      string
	BitRate    int
	Format     string
	HasVideo   bool
	HasAudio   bool
	FileSize   int64
}

// DefaultAudioExtractionOptions returns sensible defaults for audio extraction
func DefaultAudioExtractionOptions() *AudioExtractionOptions {
	return &AudioExtractionOptions{
		OutputFormat: AudioFormatMP3,
		SampleRate:   16000, // Optimal for speech recognition
		Channels:     1,     // Mono for better diarization
		Bitrate:      "128k",
	}
}

// GetMediaInfo retrieves metadata about an audio or video file using ffprobe
func GetMediaInfo(path string) (*MediaInfo, error) {
	// Check file exists
	fileInfo, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to access file: %w", err)
	}

	// Get duration
	durationCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	durationOut, err := durationCmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}
	durationSec, _ := strconv.ParseFloat(strings.TrimSpace(string(durationOut)), 64)

	// Get format info
	formatCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "format=format_name,bit_rate",
		"-of", "default=noprint_wrappers=1",
		path,
	)
	formatOut, _ := formatCmd.Output()
	formatInfo := parseFFprobeOutput(string(formatOut))

	// Get audio stream info
	audioCmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "a:0",
		"-show_entries", "stream=codec_name,sample_rate,channels",
		"-of", "default=noprint_wrappers=1",
		path,
	)
	audioOut, _ := audioCmd.Output()
	audioInfo := parseFFprobeOutput(string(audioOut))

	// Check for video stream
	videoCmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=codec_type",
		"-of", "default=noprint_wrappers=1:nokey=1",
		path,
	)
	videoOut, _ := videoCmd.Output()
	hasVideo := strings.TrimSpace(string(videoOut)) == "video"

	info := &MediaInfo{
		Path:     path,
		Duration: time.Duration(durationSec * float64(time.Second)),
		FileSize: fileInfo.Size(),
		HasVideo: hasVideo,
		HasAudio: audioInfo["codec_name"] != "",
	}

	if sr, ok := audioInfo["sample_rate"]; ok {
		info.SampleRate, _ = strconv.Atoi(sr)
	}
	if ch, ok := audioInfo["channels"]; ok {
		info.Channels, _ = strconv.Atoi(ch)
	}
	if codec, ok := audioInfo["codec_name"]; ok {
		info.Codec = codec
	}
	if br, ok := formatInfo["bit_rate"]; ok {
		info.BitRate, _ = strconv.Atoi(br)
	}
	if fmt, ok := formatInfo["format_name"]; ok {
		info.Format = fmt
	}

	return info, nil
}

// parseFFprobeOutput parses key=value output from ffprobe
func parseFFprobeOutput(output string) map[string]string {
	result := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			value := strings.TrimSpace(line[idx+1:])
			result[key] = value
		}
	}
	return result
}

// ExtractAudio extracts audio from a video file with optional conversion
// Returns the path to the extracted audio file
func ExtractAudio(inputPath string, opts *AudioExtractionOptions) (string, error) {
	if opts == nil {
		opts = DefaultAudioExtractionOptions()
	}

	// Validate sample rate
	if opts.SampleRate < 8000 || opts.SampleRate > 48000 {
		return "", fmt.Errorf("sample rate must be between 8000 and 48000 Hz")
	}

	// Generate output path
	ext := filepath.Ext(inputPath)
	baseName := strings.TrimSuffix(filepath.Base(inputPath), ext)
	outputDir := filepath.Dir(inputPath)
	outputPath := filepath.Join(outputDir, fmt.Sprintf("%s_audio.%s", baseName, opts.OutputFormat))

	// Build ffmpeg command
	args := []string{
		"-y",           // Overwrite output
		"-i", inputPath, // Input file
		"-vn", // No video
		"-ar", strconv.Itoa(opts.SampleRate), // Sample rate
		"-ac", strconv.Itoa(opts.Channels), // Channels
	}

	// Add format-specific options
	switch opts.OutputFormat {
	case AudioFormatMP3:
		args = append(args, "-codec:a", "libmp3lame", "-b:a", opts.Bitrate)
	case AudioFormatM4A:
		args = append(args, "-codec:a", "aac", "-b:a", opts.Bitrate)
	case AudioFormatWAV:
		args = append(args, "-codec:a", "pcm_s16le")
	case AudioFormatFLAC:
		args = append(args, "-codec:a", "flac")
	case AudioFormatOGG:
		args = append(args, "-codec:a", "libvorbis", "-b:a", opts.Bitrate)
	case AudioFormatWEBM:
		args = append(args, "-codec:a", "libopus", "-b:a", opts.Bitrate)
	default:
		args = append(args, "-codec:a", "libmp3lame", "-b:a", opts.Bitrate)
	}

	args = append(args, outputPath)

	cmd := exec.Command("ffmpeg", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("ffmpeg error: %w\nOutput: %s", err, string(output))
	}

	return outputPath, nil
}

// ConvertAudio converts an audio file to a different format optimized for transcription
func ConvertAudio(inputPath string, opts *AudioExtractionOptions) (string, error) {
	return ExtractAudio(inputPath, opts) // Same logic works for audio files
}

// OptimizeForTranscription prepares a media file for optimal transcription
// - Extracts audio from video files
// - Converts heavy formats (WAV/FLAC) to compressed formats
// - Ensures sample rate is in optimal range (16kHz-44.1kHz)
// Returns the path to the optimized file (may be original if already optimal)
func OptimizeForTranscription(inputPath string) (string, bool, error) {
	info, err := GetMediaInfo(inputPath)
	if err != nil {
		return "", false, fmt.Errorf("failed to analyze file: %w", err)
	}

	if !info.HasAudio {
		return "", false, fmt.Errorf("file has no audio track")
	}

	// Check if optimization is needed
	needsOptimization := false
	opts := DefaultAudioExtractionOptions()

	// Video files always need audio extraction
	if info.HasVideo {
		needsOptimization = true
	}

	// Large uncompressed formats should be converted
	ext := strings.ToLower(filepath.Ext(inputPath))
	if ext == ".wav" || ext == ".flac" || ext == ".aiff" {
		// Only convert if file is large (>50MB)
		if info.FileSize > 50*1024*1024 {
			needsOptimization = true
		}
	}

	// Sample rate outside optimal range
	if info.SampleRate > 0 && (info.SampleRate < 16000 || info.SampleRate > 44100) {
		needsOptimization = true
		if info.SampleRate < 16000 {
			opts.SampleRate = 16000
		} else {
			opts.SampleRate = 44100
		}
	}

	if !needsOptimization {
		return inputPath, false, nil
	}

	// Perform optimization
	outputPath, err := ExtractAudio(inputPath, opts)
	if err != nil {
		return "", false, err
	}

	return outputPath, true, nil
}

// IsVideoFile checks if a file is a video based on extension
func IsVideoFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	videoExts := map[string]bool{
		".mp4": true, ".mkv": true, ".mov": true, ".avi": true,
		".webm": true, ".flv": true, ".wmv": true, ".m4v": true,
		".mpeg": true, ".mpg": true, ".3gp": true, ".ts": true,
	}
	return videoExts[ext]
}

// IsAudioFile checks if a file is an audio file based on extension
func IsAudioFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	audioExts := map[string]bool{
		".mp3": true, ".m4a": true, ".wav": true, ".flac": true,
		".ogg": true, ".wma": true, ".aac": true, ".opus": true,
		".aiff": true, ".webm": true,
	}
	return audioExts[ext]
}

// IsSupportedMediaFile checks if a file is a supported audio or video format
func IsSupportedMediaFile(path string) bool {
	return IsVideoFile(path) || IsAudioFile(path)
}

// CheckFFmpeg checks if ffmpeg is installed and returns version info
func CheckFFmpeg() (string, error) {
	cmd := exec.Command("ffmpeg", "-version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("ffmpeg not found: %w\n\n%s", err, GetFFmpegInstallHelp())
	}

	// Extract first line (version info)
	lines := strings.Split(string(output), "\n")
	if len(lines) > 0 {
		return strings.TrimSpace(lines[0]), nil
	}
	return "ffmpeg installed", nil
}

// CheckFFprobe checks if ffprobe is installed
func CheckFFprobe() error {
	cmd := exec.Command("ffprobe", "-version")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffprobe not found: %w\n\n%s", err, GetFFmpegInstallHelp())
	}
	return nil
}

// GetFFmpegInstallHelp returns platform-specific installation instructions
func GetFFmpegInstallHelp() string {
	switch runtime.GOOS {
	case "darwin":
		return `Install FFmpeg on macOS:
  brew install ffmpeg

Or download from: https://ffmpeg.org/download.html`
	case "linux":
		return `Install FFmpeg on Linux:
  Ubuntu/Debian: sudo apt install ffmpeg
  Fedora:        sudo dnf install ffmpeg
  Arch:          sudo pacman -S ffmpeg

Or download from: https://ffmpeg.org/download.html`
	case "windows":
		return `Install FFmpeg on Windows:
  winget install ffmpeg

Or with Chocolatey:
  choco install ffmpeg

Or download from: https://ffmpeg.org/download.html
Then add to PATH.`
	default:
		return `Please install FFmpeg from: https://ffmpeg.org/download.html`
	}
}

// EstimateTranscriptionTime estimates how long transcription will take
// ElevenLabs processes ~1 minute of audio in ~5-10 seconds
func EstimateTranscriptionTime(duration time.Duration) time.Duration {
	// Rough estimate: 10 seconds per minute of audio, plus 5 seconds overhead
	minutes := duration.Minutes()
	return time.Duration(minutes*10)*time.Second + 5*time.Second
}

// CalculateOptimalChunkSize calculates optimal chunk size for very long files
// Files over 8 minutes are automatically chunked by ElevenLabs
func CalculateOptimalChunkSize(duration time.Duration) time.Duration {
	// ElevenLabs chunks at 8 minutes internally
	// For very long files, we might want to pre-chunk for better control
	if duration > 2*time.Hour {
		return 30 * time.Minute
	}
	if duration > 1*time.Hour {
		return 20 * time.Minute
	}
	// Let ElevenLabs handle chunking for shorter files
	return duration
}
