package scribe_test

import (
	"context"
	"fmt"
	"log"
	"time"

	"capycut/scribe"
)

// Example_basic demonstrates basic transcription usage
func Example_basic() {
	// Create client with API key
	client, err := scribe.NewClient("your-api-key")
	if err != nil {
		log.Fatal(err)
	}

	// Transcribe a file
	resp, err := client.Transcribe(context.Background(), &scribe.TranscribeRequest{
		FilePath:       "/path/to/audio.mp3",
		Model:          scribe.ModelScribeV1,
		Diarize:        boolPtr(true),
		TagAudioEvents: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Transcription:", resp.Text)
	fmt.Println("Language:", resp.LanguageCode)
}

// Example_withDiarization demonstrates speaker diarization
func Example_withDiarization() {
	client, _ := scribe.NewClient("your-api-key")

	resp, err := client.Transcribe(context.Background(), &scribe.TranscribeRequest{
		FilePath:    "/path/to/meeting.mp4",
		Model:       scribe.ModelScribeV1,
		Diarize:     boolPtr(true),
		NumSpeakers: intPtr(3), // Expected 3 speakers
	})
	if err != nil {
		log.Fatal(err)
	}

	// Parse the response into a structured transcript
	transcript := scribe.ParseResponse(resp)

	// Get speaker statistics
	stats := transcript.GetSpeakerStats()
	for speaker, duration := range stats {
		fmt.Printf("%s spoke for %v\n", speaker, duration)
	}

	// Format as a script
	script := transcript.FormatAsScript()
	fmt.Println(script)
}

// Example_subtitles demonstrates generating SRT/VTT subtitles
func Example_subtitles() {
	client, _ := scribe.NewClient("your-api-key")

	resp, err := client.Transcribe(context.Background(), &scribe.TranscribeRequest{
		FilePath:       "/path/to/video.mp4",
		TagAudioEvents: true,
	})
	if err != nil {
		log.Fatal(err)
	}

	transcript := scribe.ParseResponse(resp)

	// Generate SRT subtitles
	opts := scribe.DefaultSubtitleOptions()
	opts.Format = scribe.SubtitleFormatSRT
	opts.IncludeSpeakerLabels = true

	err = scribe.GenerateAndWriteSubtitles(transcript, "/path/to/output.srt", opts)
	if err != nil {
		log.Fatal(err)
	}

	// Generate VTT subtitles
	opts.Format = scribe.SubtitleFormatVTT
	err = scribe.GenerateAndWriteSubtitles(transcript, "/path/to/output.vtt", opts)
	if err != nil {
		log.Fatal(err)
	}
}

// Example_audioExtraction demonstrates extracting audio from video
func Example_audioExtraction() {
	// Check FFmpeg is available
	version, err := scribe.CheckFFmpeg()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("FFmpeg version:", version)

	// Get media info
	info, err := scribe.GetMediaInfo("/path/to/video.mp4")
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("Duration: %v\n", info.Duration)
	fmt.Printf("Has video: %v\n", info.HasVideo)
	fmt.Printf("Has audio: %v\n", info.HasAudio)

	// Extract and optimize audio for transcription
	optimizedPath, wasConverted, err := scribe.OptimizeForTranscription("/path/to/video.mp4")
	if err != nil {
		log.Fatal(err)
	}

	if wasConverted {
		fmt.Println("Audio extracted to:", optimizedPath)
	} else {
		fmt.Println("File was already optimized")
	}

	// Or manually extract with custom options
	opts := &scribe.AudioExtractionOptions{
		OutputFormat: scribe.AudioFormatMP3,
		SampleRate:   16000,
		Channels:     1,
		Bitrate:      "128k",
	}

	audioPath, err := scribe.ExtractAudio("/path/to/video.mp4", opts)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Audio extracted to:", audioPath)
}

// Example_transcriptAnalysis demonstrates analyzing a transcript
func Example_transcriptAnalysis() {
	// Assuming you have a transcript from a previous transcription
	transcript := &scribe.Transcript{
		Segments: []scribe.SpeakerSegment{
			{SpeakerID: "speaker_0", Text: "Hello, welcome to the meeting.", StartTime: 0, EndTime: 3 * time.Second},
			{SpeakerID: "speaker_1", Text: "Thanks for having me.", StartTime: 3 * time.Second, EndTime: 5 * time.Second},
			{SpeakerID: "speaker_0", Text: "Let's discuss the project timeline.", StartTime: 5 * time.Second, EndTime: 8 * time.Second},
		},
	}

	// Count speakers
	fmt.Printf("Number of speakers: %d\n", transcript.CountSpeakers())

	// Get dominant speaker
	dominant, duration := transcript.GetDominantSpeaker()
	fmt.Printf("Dominant speaker: %s (%v)\n", dominant, duration)

	// Search for text
	matches := transcript.SearchText("project")
	fmt.Printf("Found 'project' in %d segments\n", len(matches))

	// Get timeline
	timeline := transcript.GetTimeline()
	for _, entry := range timeline {
		fmt.Printf("[%v] %s: %s\n", entry.StartTime, entry.Speaker, entry.Text)
	}
}

// Example_retryLogic demonstrates using retry logic for unreliable networks
func Example_retryLogic() {
	client, _ := scribe.NewClient("your-api-key")

	// Use TranscribeWithRetry for automatic retry on transient failures
	resp, err := client.TranscribeWithRetry(context.Background(), &scribe.TranscribeRequest{
		FilePath: "/path/to/large-file.mp4",
	}, 4) // Max 4 retries with exponential backoff

	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Transcription:", resp.Text)
}

// Example_cloudStorage demonstrates using cloud storage URLs
func Example_cloudStorage() {
	client, _ := scribe.NewClient("your-api-key")

	// Transcribe directly from a cloud storage URL (no local file upload needed)
	resp, err := client.Transcribe(context.Background(), &scribe.TranscribeRequest{
		CloudStorageURL: "https://storage.example.com/audio.mp3",
		Model:           scribe.ModelScribeV1,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Transcription:", resp.Text)
}

// Example_webhook demonstrates async transcription with webhooks
func Example_webhook() {
	client, _ := scribe.NewClient("your-api-key")

	// For long files, use webhooks to avoid timeout
	// The request returns immediately, results are sent to your webhook endpoint
	_, err := client.Transcribe(context.Background(), &scribe.TranscribeRequest{
		FilePath:        "/path/to/long-file.mp4",
		UseWebhook:      true,
		WebhookMetadata: `{"job_id": "abc123", "user_id": "user456"}`,
	})
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("Request submitted, results will be sent to webhook")
}

// Helper functions for examples
func boolPtr(b bool) *bool     { return &b }
func intPtr(i int) *int        { return &i }
func floatPtr(f float64) *float64 { return &f }
