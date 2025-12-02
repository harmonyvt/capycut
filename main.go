package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"capycut/ai"
	"capycut/video"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
	"github.com/joho/godotenv"
)

// Build info - set via ldflags
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// Styles
var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B")).
			MarginBottom(1)

	subtitleStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#4ECDC4")).
			MarginBottom(1)

	successStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#95E1A3"))

	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF6B6B"))

	infoStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A8A8A8"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#4ECDC4")).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	capybaraLogo = `
    ╭─────────────────────────────────────╮
    │  🦫 CapyCut - AI Video Clipper      │
    ╰─────────────────────────────────────╯`
)

func main() {
	// Parse flags
	versionFlag := flag.Bool("version", false, "Print version information")
	shortVersionFlag := flag.Bool("v", false, "Print version information (short)")
	flag.Parse()

	if *versionFlag || *shortVersionFlag {
		fmt.Printf("capycut %s\n", version)
		fmt.Printf("  commit: %s\n", commit)
		fmt.Printf("  built:  %s\n", date)
		fmt.Printf("  go:     %s\n", runtime.Version())
		fmt.Printf("  os/arch: %s/%s\n", runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Load .env file if it exists (won't error if missing)
	_ = godotenv.Load()

	// Print header
	fmt.Println(titleStyle.Render(capybaraLogo))

	// Check for ffmpeg
	if err := video.CheckFFmpeg(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}
	if err := video.CheckFFprobe(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	// Check for Azure OpenAI config
	if err := ai.CheckConfig(); err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		fmt.Println(infoStyle.Render(ai.GetAPIKeyHelp()))
		os.Exit(1)
	}

	// Main loop
	for {
		if !runClipWorkflow() {
			break
		}
	}

	fmt.Println(subtitleStyle.Render("\n🦫 Thanks for using CapyCut! Bye bye!"))
}

func runClipWorkflow() bool {
	// Step 1: Select video file
	var videoPath string
	startDir, _ := os.Getwd()

	filePicker := huh.NewFilePicker().
		Title("Select a video file").
		Description("Navigate and select a video to clip").
		Picking(true).
		CurrentDirectory(startDir).
		ShowHidden(false).
		ShowPermissions(false).
		ShowSize(true).
		Height(15).
		AllowedTypes([]string{".mp4", ".mkv", ".mov", ".avi", ".webm", ".flv", ".wmv", ".m4v"}).
		Value(&videoPath)

	err := huh.NewForm(huh.NewGroup(filePicker)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return false
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return false
	}

	// Get video info
	var videoInfo *video.VideoInfo
	err = spinner.New().
		Title("Reading video information...").
		Action(func() {
			videoInfo, err = video.GetVideoInfo(videoPath)
		}).
		Run()

	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinue()
	}

	// Display video info
	infoBox := boxStyle.Render(fmt.Sprintf(
		"📹 %s\n⏱  Duration: %s",
		videoInfo.Filename,
		video.FormatDuration(videoInfo.Duration),
	))
	fmt.Println(infoBox)

	// Step 2: Get clip description
	var clipDescription string
	descInput := huh.NewText().
		Title("🤖 What would you like to clip?").
		Description("Describe in natural language, e.g.:\n• \"from 3 minutes to 5 minutes 30 seconds\"\n• \"first 2 minutes\"\n• \"start at 1:23, end at 4:56\"\n• \"last 45 seconds\"").
		Placeholder("Type your clip description here...").
		CharLimit(500).
		Value(&clipDescription)

	err = huh.NewForm(huh.NewGroup(descInput)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return askToContinue()
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinue()
	}

	// Step 3: Parse with AI
	var clipReq *ai.ClipRequest
	var parseErr error

	err = spinner.New().
		Title("🦫 Chomp chomp... understanding your request...").
		Action(func() {
			parser, err := ai.NewParser()
			if err != nil {
				parseErr = err
				return
			}
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			clipReq, parseErr = parser.ParseClipRequest(ctx, clipDescription, videoInfo.Duration)
		}).
		Run()

	if err != nil || parseErr != nil {
		if parseErr != nil {
			fmt.Println(errorStyle.Render("Error: " + parseErr.Error()))
		} else {
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
		}
		return askToContinue()
	}

	// Calculate clip duration
	clipDuration, err := video.CalculateClipDuration(clipReq.StartTime, clipReq.EndTime)
	if err != nil {
		fmt.Println(errorStyle.Render("Error calculating duration: " + err.Error()))
		return askToContinue()
	}

	// Generate output path
	outputPath := video.GenerateOutputPath(videoPath, clipReq.StartTime, clipReq.EndTime)

	// Step 4: Confirm
	summaryBox := boxStyle.Render(fmt.Sprintf(
		"📋 Clip Summary\n\n"+
			"Input:    %s\n"+
			"Start:    %s\n"+
			"End:      %s\n"+
			"Duration: %s\n"+
			"Output:   %s",
		filepath.Base(videoPath),
		clipReq.StartTime,
		clipReq.EndTime,
		video.FormatDuration(clipDuration),
		filepath.Base(outputPath),
	))
	fmt.Println(summaryBox)

	var proceed bool
	confirmSelect := huh.NewConfirm().
		Title("Proceed with this clip?").
		Affirmative("Yes, cut it!").
		Negative("No, cancel").
		Value(&proceed)

	err = huh.NewForm(huh.NewGroup(confirmSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil || !proceed {
		fmt.Println(infoStyle.Render("Clip cancelled."))
		return askToContinue()
	}

	// Step 5: Execute clip
	var clipErr error
	err = spinner.New().
		Title("🦫 Chomp chomp... clipping video...").
		Action(func() {
			params := video.ClipParams{
				InputPath:  videoPath,
				StartTime:  clipReq.StartTime,
				EndTime:    clipReq.EndTime,
				OutputPath: outputPath,
			}
			clipErr = video.ClipVideo(params)
		}).
		Run()

	if err != nil || clipErr != nil {
		if clipErr != nil {
			fmt.Println(errorStyle.Render("Error clipping video: " + clipErr.Error()))
		} else {
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
		}
		return askToContinue()
	}

	// Get output file info
	outputInfo, _ := os.Stat(outputPath)
	outputSize := "unknown"
	if outputInfo != nil {
		outputSize = formatFileSize(outputInfo.Size())
	}

	// Success!
	successBox := boxStyle.Render(fmt.Sprintf(
		"✅ Done!\n\n"+
			"Saved to: %s\n"+
			"Size: %s",
		outputPath,
		outputSize,
	))
	fmt.Println(successStyle.Render(successBox))

	return askToContinue()
}

func askToContinue() bool {
	var choice string
	selectNext := huh.NewSelect[string]().
		Title("What next?").
		Options(
			huh.NewOption("Clip another video", "another"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&choice)

	err := huh.NewForm(huh.NewGroup(selectNext)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		return false
	}

	return choice == "another"
}

func formatFileSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d bytes", bytes)
	}
}
