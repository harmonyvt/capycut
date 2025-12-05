package scribe

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// ParseResponse converts an API response into a high-level Transcript structure
// with speaker segments and audio events properly grouped
func ParseResponse(resp *TranscribeResponse) *Transcript {
	if resp == nil {
		return nil
	}

	transcript := &Transcript{
		Language: resp.LanguageCode,
		FullText: resp.Text,
		Raw:      resp,
	}

	// Calculate duration from last word
	if len(resp.Words) > 0 {
		lastWord := resp.Words[len(resp.Words)-1]
		transcript.Duration = time.Duration(lastWord.End * float64(time.Second))
	}

	// Parse segments by speaker and extract audio events
	transcript.Segments = groupWordsBySpeaker(resp.Words)
	transcript.AudioEvents = extractAudioEvents(resp.Words)

	return transcript
}

// groupWordsBySpeaker groups consecutive words by speaker into segments
func groupWordsBySpeaker(words []Word) []SpeakerSegment {
	if len(words) == 0 {
		return nil
	}

	var segments []SpeakerSegment
	var currentSegment *SpeakerSegment

	for _, word := range words {
		// Skip audio events and spacing for segment grouping
		if word.Type == WordTypeAudioEvent {
			continue
		}

		// Start new segment if speaker changes
		if currentSegment == nil || (word.SpeakerID != "" && word.SpeakerID != currentSegment.SpeakerID) {
			// Save current segment if exists
			if currentSegment != nil {
				currentSegment.Text = strings.TrimSpace(currentSegment.Text)
				segments = append(segments, *currentSegment)
			}

			// Create new segment
			speakerID := word.SpeakerID
			if speakerID == "" {
				speakerID = "speaker_0" // Default speaker
			}

			currentSegment = &SpeakerSegment{
				SpeakerID: speakerID,
				StartTime: time.Duration(word.Start * float64(time.Second)),
				Words:     []Word{},
			}
		}

		// Add word to current segment
		if word.Type == WordTypeWord {
			currentSegment.Words = append(currentSegment.Words, word)
			currentSegment.Text += word.Text
			currentSegment.EndTime = time.Duration(word.End * float64(time.Second))
		} else if word.Type == WordTypeSpacing {
			currentSegment.Text += word.Text
		}
	}

	// Don't forget the last segment
	if currentSegment != nil {
		currentSegment.Text = strings.TrimSpace(currentSegment.Text)
		segments = append(segments, *currentSegment)
	}

	return segments
}

// extractAudioEvents extracts all audio events from the words list
func extractAudioEvents(words []Word) []AudioEvent {
	var events []AudioEvent

	for _, word := range words {
		if word.Type == WordTypeAudioEvent {
			events = append(events, AudioEvent{
				Description: word.Text,
				StartTime:   time.Duration(word.Start * float64(time.Second)),
				EndTime:     time.Duration(word.End * float64(time.Second)),
			})
		}
	}

	return events
}

// FormatAsScript formats the transcript as a readable script with speaker labels
func (t *Transcript) FormatAsScript() string {
	if t == nil || len(t.Segments) == 0 {
		return ""
	}

	var sb strings.Builder
	var lastSpeaker string

	for _, segment := range t.Segments {
		// Add speaker label when it changes
		if segment.SpeakerID != lastSpeaker {
			if sb.Len() > 0 {
				sb.WriteString("\n\n")
			}
			sb.WriteString(formatSpeakerLabel(segment.SpeakerID))
			sb.WriteString(":\n")
			lastSpeaker = segment.SpeakerID
		}

		sb.WriteString(segment.Text)
		sb.WriteString(" ")
	}

	return strings.TrimSpace(sb.String())
}

// FormatAsScriptWithTimestamps formats the transcript with timestamps
func (t *Transcript) FormatAsScriptWithTimestamps() string {
	if t == nil || len(t.Segments) == 0 {
		return ""
	}

	var sb strings.Builder

	for _, segment := range t.Segments {
		// Format: [00:00:00 - 00:00:10] Speaker 1:
		sb.WriteString(fmt.Sprintf("[%s - %s] %s:\n",
			formatTimestamp(segment.StartTime),
			formatTimestamp(segment.EndTime),
			formatSpeakerLabel(segment.SpeakerID)))
		sb.WriteString(segment.Text)
		sb.WriteString("\n\n")
	}

	return strings.TrimSpace(sb.String())
}

// FormatAsJSON returns a JSON-like formatted string (for debugging)
func (t *Transcript) FormatAsJSON() string {
	if t == nil {
		return "{}"
	}

	var sb strings.Builder
	sb.WriteString("{\n")
	sb.WriteString(fmt.Sprintf("  \"language\": %q,\n", t.Language))
	sb.WriteString(fmt.Sprintf("  \"duration\": %q,\n", t.Duration.String()))
	sb.WriteString(fmt.Sprintf("  \"speakers\": %d,\n", t.CountSpeakers()))
	sb.WriteString(fmt.Sprintf("  \"segments\": %d,\n", len(t.Segments)))
	sb.WriteString(fmt.Sprintf("  \"audio_events\": %d,\n", len(t.AudioEvents)))
	sb.WriteString(fmt.Sprintf("  \"text\": %q\n", truncateString(t.FullText, 200)))
	sb.WriteString("}")

	return sb.String()
}

// GetSpeakerText returns all text spoken by a specific speaker
func (t *Transcript) GetSpeakerText(speakerID string) string {
	if t == nil {
		return ""
	}

	var parts []string
	for _, segment := range t.Segments {
		if segment.SpeakerID == speakerID {
			parts = append(parts, segment.Text)
		}
	}

	return strings.Join(parts, " ")
}

// GetSpeakers returns a list of unique speaker IDs
func (t *Transcript) GetSpeakers() []string {
	if t == nil {
		return nil
	}

	seen := make(map[string]bool)
	var speakers []string

	for _, segment := range t.Segments {
		if !seen[segment.SpeakerID] {
			seen[segment.SpeakerID] = true
			speakers = append(speakers, segment.SpeakerID)
		}
	}

	return speakers
}

// CountSpeakers returns the number of unique speakers
func (t *Transcript) CountSpeakers() int {
	return len(t.GetSpeakers())
}

// GetTextBetween returns the transcript text between two timestamps
func (t *Transcript) GetTextBetween(start, end time.Duration) string {
	if t == nil || t.Raw == nil {
		return ""
	}

	var parts []string
	for _, word := range t.Raw.Words {
		wordStart := time.Duration(word.Start * float64(time.Second))
		wordEnd := time.Duration(word.End * float64(time.Second))

		// Word overlaps with the requested range
		if wordEnd >= start && wordStart <= end {
			if word.Type == WordTypeWord {
				parts = append(parts, word.Text)
			} else if word.Type == WordTypeAudioEvent {
				parts = append(parts, fmt.Sprintf("[%s]", word.Text))
			}
		}
	}

	return strings.Join(parts, " ")
}

// GetWordsInRange returns words within a time range
func (t *Transcript) GetWordsInRange(start, end time.Duration) []Word {
	if t == nil || t.Raw == nil {
		return nil
	}

	var words []Word
	for _, word := range t.Raw.Words {
		wordStart := time.Duration(word.Start * float64(time.Second))
		wordEnd := time.Duration(word.End * float64(time.Second))

		if wordEnd >= start && wordStart <= end {
			words = append(words, word)
		}
	}

	return words
}

// SplitByDuration splits the transcript into chunks of approximately the given duration
func (t *Transcript) SplitByDuration(chunkDuration time.Duration) []SpeakerSegment {
	if t == nil || len(t.Segments) == 0 {
		return nil
	}

	var chunks []SpeakerSegment
	var currentChunk *SpeakerSegment

	for _, segment := range t.Segments {
		if currentChunk == nil {
			currentChunk = &SpeakerSegment{
				SpeakerID: segment.SpeakerID,
				StartTime: segment.StartTime,
			}
		}

		// Check if adding this segment would exceed chunk duration
		if segment.EndTime-currentChunk.StartTime > chunkDuration && currentChunk.Text != "" {
			currentChunk.Text = strings.TrimSpace(currentChunk.Text)
			chunks = append(chunks, *currentChunk)

			currentChunk = &SpeakerSegment{
				SpeakerID: segment.SpeakerID,
				StartTime: segment.StartTime,
			}
		}

		// Add segment to current chunk
		if currentChunk.Text != "" && segment.SpeakerID != currentChunk.SpeakerID {
			currentChunk.Text += "\n" + formatSpeakerLabel(segment.SpeakerID) + ": "
		}
		currentChunk.Text += segment.Text + " "
		currentChunk.EndTime = segment.EndTime
		currentChunk.Words = append(currentChunk.Words, segment.Words...)
	}

	// Don't forget the last chunk
	if currentChunk != nil && currentChunk.Text != "" {
		currentChunk.Text = strings.TrimSpace(currentChunk.Text)
		chunks = append(chunks, *currentChunk)
	}

	return chunks
}

// MergeShortSegments merges segments shorter than the given threshold
func (t *Transcript) MergeShortSegments(minDuration time.Duration) []SpeakerSegment {
	if t == nil || len(t.Segments) == 0 {
		return nil
	}

	var merged []SpeakerSegment

	for i, segment := range t.Segments {
		duration := segment.EndTime - segment.StartTime

		if duration < minDuration && len(merged) > 0 {
			// Try to merge with previous segment if same speaker
			last := &merged[len(merged)-1]
			if last.SpeakerID == segment.SpeakerID {
				last.Text = last.Text + " " + segment.Text
				last.EndTime = segment.EndTime
				last.Words = append(last.Words, segment.Words...)
				continue
			}

			// Try to merge with next segment if same speaker
			if i+1 < len(t.Segments) && t.Segments[i+1].SpeakerID == segment.SpeakerID {
				// Skip, will be merged with next
				merged = append(merged, segment)
				continue
			}
		}

		merged = append(merged, segment)
	}

	return merged
}

// GetWordConfidenceStats returns statistics about word confidence scores
func (t *Transcript) GetWordConfidenceStats() (min, max, avg float64) {
	if t == nil || t.Raw == nil || len(t.Raw.Words) == 0 {
		return 0, 0, 0
	}

	min = 1.0
	max = 0.0
	var sum float64
	var count int

	for _, word := range t.Raw.Words {
		if word.Type == WordTypeWord && word.Confidence > 0 {
			if word.Confidence < min {
				min = word.Confidence
			}
			if word.Confidence > max {
				max = word.Confidence
			}
			sum += word.Confidence
			count++
		}
	}

	if count > 0 {
		avg = sum / float64(count)
	}

	return min, max, avg
}

// GetLowConfidenceWords returns words with confidence below the threshold
func (t *Transcript) GetLowConfidenceWords(threshold float64) []Word {
	if t == nil || t.Raw == nil {
		return nil
	}

	var lowConf []Word
	for _, word := range t.Raw.Words {
		if word.Type == WordTypeWord && word.Confidence > 0 && word.Confidence < threshold {
			lowConf = append(lowConf, word)
		}
	}

	return lowConf
}

// SortSegmentsBySpeaker groups all segments by speaker
func (t *Transcript) SortSegmentsBySpeaker() map[string][]SpeakerSegment {
	if t == nil {
		return nil
	}

	result := make(map[string][]SpeakerSegment)
	for _, segment := range t.Segments {
		result[segment.SpeakerID] = append(result[segment.SpeakerID], segment)
	}

	return result
}

// GetSpeakerStats returns speaking time statistics per speaker
func (t *Transcript) GetSpeakerStats() map[string]time.Duration {
	if t == nil {
		return nil
	}

	stats := make(map[string]time.Duration)
	for _, segment := range t.Segments {
		duration := segment.EndTime - segment.StartTime
		stats[segment.SpeakerID] += duration
	}

	return stats
}

// GetDominantSpeaker returns the speaker with the most speaking time
func (t *Transcript) GetDominantSpeaker() (string, time.Duration) {
	stats := t.GetSpeakerStats()
	if stats == nil {
		return "", 0
	}

	var dominant string
	var maxDuration time.Duration

	for speaker, duration := range stats {
		if duration > maxDuration {
			dominant = speaker
			maxDuration = duration
		}
	}

	return dominant, maxDuration
}

// SearchText finds segments containing the search term
func (t *Transcript) SearchText(query string) []SpeakerSegment {
	if t == nil {
		return nil
	}

	query = strings.ToLower(query)
	var matches []SpeakerSegment

	for _, segment := range t.Segments {
		if strings.Contains(strings.ToLower(segment.Text), query) {
			matches = append(matches, segment)
		}
	}

	return matches
}

// GetTimeline returns a chronological timeline of speakers and events
type TimelineEntry struct {
	Type      string // "speech" or "event"
	Speaker   string
	Text      string
	StartTime time.Duration
	EndTime   time.Duration
}

func (t *Transcript) GetTimeline() []TimelineEntry {
	if t == nil {
		return nil
	}

	var entries []TimelineEntry

	// Add segments
	for _, segment := range t.Segments {
		entries = append(entries, TimelineEntry{
			Type:      "speech",
			Speaker:   segment.SpeakerID,
			Text:      segment.Text,
			StartTime: segment.StartTime,
			EndTime:   segment.EndTime,
		})
	}

	// Add audio events
	for _, event := range t.AudioEvents {
		entries = append(entries, TimelineEntry{
			Type:      "event",
			Text:      event.Description,
			StartTime: event.StartTime,
			EndTime:   event.EndTime,
		})
	}

	// Sort by start time
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartTime < entries[j].StartTime
	})

	return entries
}

// Helper functions

func formatSpeakerLabel(speakerID string) string {
	// Convert speaker_0 to Speaker 1, etc.
	if strings.HasPrefix(speakerID, "speaker_") {
		num := strings.TrimPrefix(speakerID, "speaker_")
		if n, err := fmt.Sscanf(num, "%d", new(int)); err == nil && n == 1 {
			var idx int
			fmt.Sscanf(num, "%d", &idx)
			return fmt.Sprintf("Speaker %d", idx+1)
		}
	}
	return speakerID
}

func formatTimestamp(d time.Duration) string {
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60

	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
