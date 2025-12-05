// Package scribe provides a Go client for the ElevenLabs Scribe speech-to-text API.
// It supports both batch file transcription (scribe_v1) and realtime streaming (scribe_v2).
package scribe

import "time"

// Model IDs for ElevenLabs Scribe API
const (
	ModelScribeV1            = "scribe_v1"
	ModelScribeV1Experimental = "scribe_v1_experimental"
	ModelScribeV2Realtime    = "scribe_v2_realtime"
)

// WordType represents the type of a transcribed element
type WordType string

const (
	WordTypeWord       WordType = "word"
	WordTypeSpacing    WordType = "spacing"
	WordTypeAudioEvent WordType = "audio_event"
)

// TranscribeRequest represents the configuration for a transcription request
type TranscribeRequest struct {
	// FilePath is the local path to the audio/video file to transcribe
	FilePath string

	// CloudStorageURL is an alternative to FilePath - a URL to the file in cloud storage
	CloudStorageURL string

	// Model is the model ID to use (scribe_v1, scribe_v1_experimental)
	// Defaults to scribe_v1 if not specified
	Model string

	// Language is an ISO-639-1 or ISO-639-3 language code
	// If not specified, language is auto-detected
	Language string

	// Diarize enables speaker diarization (who said what)
	// Defaults to true
	Diarize *bool

	// NumSpeakers is the expected number of speakers (max 32)
	// If not specified, auto-detected
	NumSpeakers *int

	// DiarizationThreshold controls speaker separation (0.1-0.4)
	// Higher = fewer speakers detected, lower = more speakers
	// Only valid when Diarize=true and NumSpeakers is not set
	DiarizationThreshold *float64

	// TagAudioEvents enables detection of non-speech sounds like [laughter], [applause]
	TagAudioEvents bool

	// Temperature controls transcription randomness (0.0-2.0)
	// Lower = more deterministic, defaults to model default (~0)
	Temperature *float64

	// UseWebhook if true, returns immediately and sends result via webhook
	UseWebhook bool

	// WebhookMetadata is optional JSON metadata for webhook tracking (max 16KB, depth 2)
	WebhookMetadata string

	// EnableLogging if false, enables zero retention mode (enterprise only)
	EnableLogging *bool
}

// TranscribeResponse represents the API response from a transcription request
type TranscribeResponse struct {
	// LanguageCode is the detected or specified language (ISO-639-1)
	LanguageCode string `json:"language_code"`

	// LanguageProbability is the confidence of language detection (0-1)
	LanguageProbability float64 `json:"language_probability"`

	// Text is the full transcribed text
	Text string `json:"text"`

	// Words contains word-level transcription details with timestamps
	Words []Word `json:"words"`

	// Transcripts contains per-channel transcripts when using multichannel mode
	// Only populated when use_multi_channel is true
	Transcripts map[string]ChannelTranscript `json:"transcripts,omitempty"`
}

// Word represents a single transcribed word or audio event
type Word struct {
	// Text is the transcribed word or audio event description
	Text string `json:"text"`

	// Type is the element type: "word", "spacing", or "audio_event"
	Type WordType `json:"type"`

	// Start is the start time in seconds
	Start float64 `json:"start"`

	// End is the end time in seconds
	End float64 `json:"end"`

	// SpeakerID identifies the speaker (when diarization is enabled)
	SpeakerID string `json:"speaker_id,omitempty"`

	// ChannelIndex identifies the channel (when multichannel mode is used)
	ChannelIndex *int `json:"channel_index,omitempty"`

	// Confidence is the model's confidence in this word (0-1)
	Confidence float64 `json:"confidence,omitempty"`
}

// ChannelTranscript represents transcription for a single audio channel
type ChannelTranscript struct {
	LanguageCode        string  `json:"language_code"`
	LanguageProbability float64 `json:"language_probability"`
	Text                string  `json:"text"`
	Words               []Word  `json:"words"`
}

// SpeakerSegment represents a contiguous segment of speech by a single speaker
type SpeakerSegment struct {
	SpeakerID string
	Text      string
	StartTime time.Duration
	EndTime   time.Duration
	Words     []Word
}

// AudioEvent represents a detected non-speech audio event
type AudioEvent struct {
	Description string
	StartTime   time.Duration
	EndTime     time.Duration
}

// Transcript is a high-level representation of a transcription result
type Transcript struct {
	// Language is the detected language
	Language string

	// FullText is the complete transcription
	FullText string

	// Segments groups words by speaker
	Segments []SpeakerSegment

	// AudioEvents contains detected non-speech events
	AudioEvents []AudioEvent

	// Duration is the total duration of the audio
	Duration time.Duration

	// Raw contains the original API response
	Raw *TranscribeResponse
}

// SubtitleFormat specifies the output format for subtitles
type SubtitleFormat string

const (
	SubtitleFormatSRT SubtitleFormat = "srt"
	SubtitleFormatVTT SubtitleFormat = "vtt"
)

// SubtitleOptions configures subtitle generation
type SubtitleOptions struct {
	// Format is the output format (srt or vtt)
	Format SubtitleFormat

	// MaxLineLength limits characters per subtitle line (default 42)
	MaxLineLength int

	// MaxDuration limits subtitle display duration (default 7s)
	MaxDuration time.Duration

	// IncludeSpeakerLabels adds speaker names to subtitles
	IncludeSpeakerLabels bool

	// IncludeAudioEvents includes audio events like [laughter]
	IncludeAudioEvents bool
}

// RealtimeConfig configures the WebSocket realtime transcription
type RealtimeConfig struct {
	// Model is the model ID (scribe_v2_realtime)
	Model string

	// Language is an ISO-639-1 language code for the input audio
	Language string

	// SampleRate is the audio sample rate (default 16000)
	SampleRate int

	// Encoding is the audio encoding format
	Encoding string

	// OnTranscript is called when a transcript segment is received
	OnTranscript func(text string, isFinal bool)

	// OnError is called when an error occurs
	OnError func(err error)
}

// AudioFormat represents supported audio formats
type AudioFormat string

const (
	AudioFormatMP3  AudioFormat = "mp3"
	AudioFormatM4A  AudioFormat = "m4a"
	AudioFormatWAV  AudioFormat = "wav"
	AudioFormatFLAC AudioFormat = "flac"
	AudioFormatOGG  AudioFormat = "ogg"
	AudioFormatWEBM AudioFormat = "webm"
)

// AudioExtractionOptions configures audio extraction from video
type AudioExtractionOptions struct {
	// OutputFormat is the target audio format (default mp3)
	OutputFormat AudioFormat

	// SampleRate is the target sample rate (default 16000, range 16000-44100)
	SampleRate int

	// Channels is the number of audio channels (default 1 for mono)
	Channels int

	// Bitrate is the target bitrate for lossy formats (default "128k")
	Bitrate string
}

// APIError represents an error response from the ElevenLabs API
type APIError struct {
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
	Code       string `json:"code,omitempty"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return e.Code + ": " + e.Message
	}
	return e.Message
}
