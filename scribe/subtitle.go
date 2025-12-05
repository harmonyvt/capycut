package scribe

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// DefaultSubtitleOptions returns sensible defaults for subtitle generation
func DefaultSubtitleOptions() *SubtitleOptions {
	return &SubtitleOptions{
		Format:               SubtitleFormatSRT,
		MaxLineLength:        42, // Standard for subtitles
		MaxDuration:          7 * time.Second,
		IncludeSpeakerLabels: true,
		IncludeAudioEvents:   true,
	}
}

// SubtitleEntry represents a single subtitle
type SubtitleEntry struct {
	Index     int
	StartTime time.Duration
	EndTime   time.Duration
	Text      string
	Speaker   string
}

// GenerateSubtitles creates subtitles from a transcript
func GenerateSubtitles(transcript *Transcript, opts *SubtitleOptions) ([]SubtitleEntry, error) {
	if transcript == nil || transcript.Raw == nil {
		return nil, fmt.Errorf("transcript is nil or has no raw data")
	}

	if opts == nil {
		opts = DefaultSubtitleOptions()
	}

	// Build subtitle entries from words
	entries := buildSubtitleEntries(transcript.Raw.Words, opts)

	// Number entries
	for i := range entries {
		entries[i].Index = i + 1
	}

	return entries, nil
}

// buildSubtitleEntries groups words into subtitle-sized chunks
func buildSubtitleEntries(words []Word, opts *SubtitleOptions) []SubtitleEntry {
	var entries []SubtitleEntry
	var currentEntry *SubtitleEntry
	var currentLineLength int

	for _, word := range words {
		// Handle audio events
		if word.Type == WordTypeAudioEvent {
			if !opts.IncludeAudioEvents {
				continue
			}

			// Audio events get their own entry
			if currentEntry != nil {
				entries = append(entries, *currentEntry)
				currentEntry = nil
				currentLineLength = 0
			}

			entries = append(entries, SubtitleEntry{
				StartTime: time.Duration(word.Start * float64(time.Second)),
				EndTime:   time.Duration(word.End * float64(time.Second)),
				Text:      fmt.Sprintf("[%s]", word.Text),
			})
			continue
		}

		// Skip spacing for subtitle purposes (we handle spacing ourselves)
		if word.Type == WordTypeSpacing {
			continue
		}

		// Check if we need to start a new entry
		startNew := currentEntry == nil

		// Start new entry if duration exceeds max
		if currentEntry != nil {
			currentDuration := time.Duration(word.End*float64(time.Second)) - currentEntry.StartTime
			if currentDuration > opts.MaxDuration {
				startNew = true
			}
		}

		// Start new entry if line length exceeds max
		if currentEntry != nil && currentLineLength+len(word.Text)+1 > opts.MaxLineLength {
			startNew = true
		}

		// Start new entry if speaker changes and we want speaker labels
		if currentEntry != nil && opts.IncludeSpeakerLabels && word.SpeakerID != "" && word.SpeakerID != currentEntry.Speaker {
			startNew = true
		}

		if startNew {
			if currentEntry != nil {
				currentEntry.Text = strings.TrimSpace(currentEntry.Text)
				entries = append(entries, *currentEntry)
			}

			currentEntry = &SubtitleEntry{
				StartTime: time.Duration(word.Start * float64(time.Second)),
				Speaker:   word.SpeakerID,
			}
			currentLineLength = 0
		}

		// Add word to current entry
		if currentLineLength > 0 {
			currentEntry.Text += " "
			currentLineLength++
		}
		currentEntry.Text += word.Text
		currentLineLength += len(word.Text)
		currentEntry.EndTime = time.Duration(word.End * float64(time.Second))
	}

	// Don't forget the last entry
	if currentEntry != nil {
		currentEntry.Text = strings.TrimSpace(currentEntry.Text)
		entries = append(entries, *currentEntry)
	}

	return entries
}

// FormatSRT formats subtitles as SRT (SubRip Text)
func FormatSRT(entries []SubtitleEntry, opts *SubtitleOptions) string {
	if opts == nil {
		opts = DefaultSubtitleOptions()
	}

	var sb strings.Builder

	for i, entry := range entries {
		// Entry number
		sb.WriteString(fmt.Sprintf("%d\n", i+1))

		// Timestamps: HH:MM:SS,mmm --> HH:MM:SS,mmm
		sb.WriteString(fmt.Sprintf("%s --> %s\n",
			formatSRTTimestamp(entry.StartTime),
			formatSRTTimestamp(entry.EndTime)))

		// Text (with optional speaker label)
		text := entry.Text
		if opts.IncludeSpeakerLabels && entry.Speaker != "" {
			text = fmt.Sprintf("[%s] %s", formatSpeakerLabel(entry.Speaker), text)
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	return strings.TrimSpace(sb.String())
}

// FormatVTT formats subtitles as WebVTT
func FormatVTT(entries []SubtitleEntry, opts *SubtitleOptions) string {
	if opts == nil {
		opts = DefaultSubtitleOptions()
	}

	var sb strings.Builder

	// VTT header
	sb.WriteString("WEBVTT\n")
	sb.WriteString("Kind: captions\n")
	sb.WriteString("Language: en\n\n")

	for i, entry := range entries {
		// Optional cue identifier
		sb.WriteString(fmt.Sprintf("%d\n", i+1))

		// Timestamps: HH:MM:SS.mmm --> HH:MM:SS.mmm
		sb.WriteString(fmt.Sprintf("%s --> %s\n",
			formatVTTTimestamp(entry.StartTime),
			formatVTTTimestamp(entry.EndTime)))

		// Text (with optional speaker label using VTT voice tags)
		text := entry.Text
		if opts.IncludeSpeakerLabels && entry.Speaker != "" {
			text = fmt.Sprintf("<v %s>%s</v>", formatSpeakerLabel(entry.Speaker), text)
		}
		sb.WriteString(text)
		sb.WriteString("\n\n")
	}

	return strings.TrimSpace(sb.String())
}

// FormatSubtitles formats subtitles in the specified format
func FormatSubtitles(entries []SubtitleEntry, opts *SubtitleOptions) string {
	if opts == nil {
		opts = DefaultSubtitleOptions()
	}

	switch opts.Format {
	case SubtitleFormatVTT:
		return FormatVTT(entries, opts)
	case SubtitleFormatSRT:
		fallthrough
	default:
		return FormatSRT(entries, opts)
	}
}

// WriteSubtitleFile writes subtitles to a file
func WriteSubtitleFile(entries []SubtitleEntry, path string, opts *SubtitleOptions) error {
	content := FormatSubtitles(entries, opts)
	return os.WriteFile(path, []byte(content), 0644)
}

// GenerateAndWriteSubtitles is a convenience function that generates and writes subtitles
func GenerateAndWriteSubtitles(transcript *Transcript, outputPath string, opts *SubtitleOptions) error {
	entries, err := GenerateSubtitles(transcript, opts)
	if err != nil {
		return fmt.Errorf("failed to generate subtitles: %w", err)
	}

	return WriteSubtitleFile(entries, outputPath, opts)
}

// formatSRTTimestamp formats a duration as SRT timestamp (HH:MM:SS,mmm)
func formatSRTTimestamp(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
}

// formatVTTTimestamp formats a duration as VTT timestamp (HH:MM:SS.mmm)
func formatVTTTimestamp(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	ms := int(d.Milliseconds()) % 1000

	return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
}

// AlignSubtitlesToVideo adjusts subtitle timestamps to account for video offset
// Useful when audio was extracted from a portion of a video
func AlignSubtitlesToVideo(entries []SubtitleEntry, offset time.Duration) []SubtitleEntry {
	aligned := make([]SubtitleEntry, len(entries))
	copy(aligned, entries)

	for i := range aligned {
		aligned[i].StartTime += offset
		aligned[i].EndTime += offset
	}

	return aligned
}

// MergeSubtitleEntries merges very short consecutive entries
func MergeSubtitleEntries(entries []SubtitleEntry, minDuration time.Duration) []SubtitleEntry {
	if len(entries) == 0 {
		return entries
	}

	var merged []SubtitleEntry

	for i := 0; i < len(entries); i++ {
		entry := entries[i]
		duration := entry.EndTime - entry.StartTime

		// If entry is too short, try to merge with next
		if duration < minDuration && i+1 < len(entries) {
			next := entries[i+1]
			// Merge if same speaker or no speaker labels
			if entry.Speaker == next.Speaker || entry.Speaker == "" || next.Speaker == "" {
				merged = append(merged, SubtitleEntry{
					Index:     len(merged) + 1,
					StartTime: entry.StartTime,
					EndTime:   next.EndTime,
					Text:      entry.Text + " " + next.Text,
					Speaker:   entry.Speaker,
				})
				i++ // Skip next entry
				continue
			}
		}

		entry.Index = len(merged) + 1
		merged = append(merged, entry)
	}

	return merged
}

// SplitLongLines splits subtitle entries that exceed max line length into multiple lines
func SplitLongLines(entries []SubtitleEntry, maxLineLength int) []SubtitleEntry {
	var result []SubtitleEntry

	for _, entry := range entries {
		if len(entry.Text) <= maxLineLength {
			result = append(result, entry)
			continue
		}

		// Split into multiple lines within the same entry
		lines := splitTextIntoLines(entry.Text, maxLineLength)
		entry.Text = strings.Join(lines, "\n")
		result = append(result, entry)
	}

	// Renumber
	for i := range result {
		result[i].Index = i + 1
	}

	return result
}

// splitTextIntoLines splits text into lines of max length, breaking at word boundaries
func splitTextIntoLines(text string, maxLength int) []string {
	words := strings.Fields(text)
	if len(words) == 0 {
		return []string{text}
	}

	var lines []string
	var currentLine strings.Builder

	for _, word := range words {
		if currentLine.Len() == 0 {
			currentLine.WriteString(word)
		} else if currentLine.Len()+1+len(word) <= maxLength {
			currentLine.WriteString(" ")
			currentLine.WriteString(word)
		} else {
			lines = append(lines, currentLine.String())
			currentLine.Reset()
			currentLine.WriteString(word)
		}
	}

	if currentLine.Len() > 0 {
		lines = append(lines, currentLine.String())
	}

	return lines
}

// SubtitleStats contains statistics about subtitles
type SubtitleStats struct {
	TotalEntries   int
	TotalDuration  time.Duration
	AverageDuration time.Duration
	LongestEntry   time.Duration
	ShortestEntry  time.Duration
	TotalCharacters int
	AverageLength  int
}

// GetSubtitleStats calculates statistics for subtitle entries
func GetSubtitleStats(entries []SubtitleEntry) SubtitleStats {
	if len(entries) == 0 {
		return SubtitleStats{}
	}

	stats := SubtitleStats{
		TotalEntries:  len(entries),
		ShortestEntry: 24 * time.Hour, // Start with a high value
	}

	for _, entry := range entries {
		duration := entry.EndTime - entry.StartTime
		stats.TotalDuration += duration
		stats.TotalCharacters += len(entry.Text)

		if duration > stats.LongestEntry {
			stats.LongestEntry = duration
		}
		if duration < stats.ShortestEntry {
			stats.ShortestEntry = duration
		}
	}

	if stats.TotalEntries > 0 {
		stats.AverageDuration = stats.TotalDuration / time.Duration(stats.TotalEntries)
		stats.AverageLength = stats.TotalCharacters / stats.TotalEntries
	}

	return stats
}
