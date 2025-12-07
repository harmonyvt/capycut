package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"capycut/gemini"
	"capycut/tui"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
)

// TranscribeOptions holds the configuration for image transcription
type TranscribeOptions struct {
	Images                   []string
	OutputDir                string
	Model                    string
	Language                 string
	DetectChapters           bool
	CombinePages             bool
	PreserveFormatting       bool
	IncludeImageDescriptions bool
	AddFrontMatter           bool
	AddTableOfContents       bool
	CreateIndexFile          bool
	Overwrite                bool
}

// runTranscribeWorkflowNew runs the new Bubble Tea based transcription UI
func runTranscribeWorkflowNew() bool {
	continueApp, err := tui.RunTranscribeUI()
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinueTranscribe()
	}
	return continueApp
}

// runTranscribeWorkflow runs the interactive image transcription workflow
// Set USE_NEW_TUI=1 environment variable to use the new Bubble Tea UI
func runTranscribeWorkflow() bool {
	// Check if user wants to use the new TUI
	if os.Getenv("USE_NEW_TUI") == "1" || os.Getenv("CAPYCUT_NEW_UI") == "1" {
		return runTranscribeWorkflowNew()
	}

	// Step 1: Select images
	var imageSources []string

	fmt.Println(subtitleStyle.Render("\n📸 Image to Markdown Transcription\n"))

	// Ask for input method
	var inputMethod string
	inputSelect := huh.NewSelect[string]().
		Title("How would you like to select images?").
		Options(
			huh.NewOption("Select a folder containing images", "folder"),
			huh.NewOption("Enter file paths or glob pattern", "pattern"),
		).
		Value(&inputMethod)

	err := huh.NewForm(huh.NewGroup(inputSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return false
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinueTranscribe()
	}

	if inputMethod == "folder" {
		// Use folder picker
		var folderPath string
		startDir, _ := os.Getwd()

		folderPicker := huh.NewFilePicker().
			Title("Select a folder containing images").
			Description("Navigate to the folder with your images (JPG, PNG, etc.)").
			CurrentDirectory(startDir).
			ShowHidden(false).
			ShowSize(true).
			DirAllowed(true).
			FileAllowed(false).
			Height(15).
			Value(&folderPath)

		err = huh.NewForm(huh.NewGroup(folderPicker)).
			WithTheme(huh.ThemeCatppuccin()).
			Run()

		if err != nil {
			if err == huh.ErrUserAborted {
				return askToContinueTranscribe()
			}
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
			return askToContinueTranscribe()
		}

		imageSources = []string{folderPath}
	} else {
		// Enter pattern manually
		var pattern string
		patternInput := huh.NewInput().
			Title("Enter file paths or glob pattern").
			Description("Examples:\n  • /path/to/images/*.png\n  • ./scans/page_*.jpg\n  • /path/to/folder").
			Placeholder("./images/*.png").
			Value(&pattern)

		err = huh.NewForm(huh.NewGroup(patternInput)).
			WithTheme(huh.ThemeCatppuccin()).
			Run()

		if err != nil {
			if err == huh.ErrUserAborted {
				return askToContinueTranscribe()
			}
			fmt.Println(errorStyle.Render("Error: " + err.Error()))
			return askToContinueTranscribe()
		}

		imageSources = []string{pattern}
	}

	// Load and validate images
	var images []string
	var loadErr error

	err = spinner.New().
		Title("Loading images...").
		Action(func() {
			images, loadErr = gemini.LoadImages(imageSources)
		}).
		Run()

	if loadErr != nil {
		fmt.Println(errorStyle.Render("Error loading images: " + loadErr.Error()))
		return askToContinueTranscribe()
	}

	// Display image info
	totalSize, count, _ := gemini.GetImageStats(images)
	infoBox := boxStyle.Render(fmt.Sprintf(
		"📷 Found %d images\n📦 Total size: %s\n\nFirst: %s\nLast:  %s",
		count,
		gemini.FormatSize(totalSize),
		filepath.Base(images[0]),
		filepath.Base(images[len(images)-1]),
	))
	fmt.Println(infoBox)

	// Warn if many images
	if len(images) > 50 {
		fmt.Println(infoStyle.Render(fmt.Sprintf(
			"⚠️  You have %d images. Processing may take a while and use significant API quota.",
			len(images),
		)))
	}

	// Step 2: Configure options
	var opts TranscribeOptions
	opts.Images = images

	// Output directory
	var outputDir string
	outputInput := huh.NewInput().
		Title("Output directory").
		Description("Where to save the generated Markdown files").
		Placeholder("./output").
		Value(&outputDir)

	// Model selection
	var modelChoice string
	modelSelect := huh.NewSelect[string]().
		Title("Select Gemini model").
		Options(
			huh.NewOption("Gemini 3 Pro (Latest, most capable)", gemini.ModelGemini3Pro),
			huh.NewOption("Gemini 3 Pro Thinking (Enhanced reasoning)", gemini.ModelGemini3ProThinking),
			huh.NewOption("Gemini 2.5 Flash (Fast)", gemini.ModelGemini25Flash),
			huh.NewOption("Gemini 2.5 Pro (Previous generation)", gemini.ModelGemini25Pro),
		).
		Value(&modelChoice)

	// Document organization
	var docOrg string
	orgSelect := huh.NewSelect[string]().
		Title("How should the document be organized?").
		Options(
			huh.NewOption("Detect chapters automatically", "chapters"),
			huh.NewOption("Combine all pages into one file", "combine"),
			huh.NewOption("Create one file per page", "pages"),
		).
		Value(&docOrg)

	// Additional options
	var additionalOpts []string
	additionalSelect := huh.NewMultiSelect[string]().
		Title("Additional options").
		Options(
			huh.NewOption("Preserve original formatting (tables, lists, etc.)", "formatting"),
			huh.NewOption("Include image/figure descriptions", "images"),
			huh.NewOption("Add YAML front matter", "frontmatter"),
			huh.NewOption("Add table of contents", "toc"),
			huh.NewOption("Create index file (for multiple documents)", "index"),
			huh.NewOption("Overwrite existing files", "overwrite"),
		).
		Value(&additionalOpts)

	err = huh.NewForm(
		huh.NewGroup(outputInput, modelSelect),
		huh.NewGroup(orgSelect, additionalSelect),
	).WithTheme(huh.ThemeCatppuccin()).Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return askToContinueTranscribe()
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinueTranscribe()
	}

	// Apply options
	if outputDir == "" {
		outputDir = "./output"
	}
	opts.OutputDir = outputDir
	opts.Model = modelChoice

	switch docOrg {
	case "chapters":
		opts.DetectChapters = true
	case "combine":
		opts.CombinePages = true
	}

	for _, opt := range additionalOpts {
		switch opt {
		case "formatting":
			opts.PreserveFormatting = true
		case "images":
			opts.IncludeImageDescriptions = true
		case "frontmatter":
			opts.AddFrontMatter = true
		case "toc":
			opts.AddTableOfContents = true
		case "index":
			opts.CreateIndexFile = true
		case "overwrite":
			opts.Overwrite = true
		}
	}

	// Step 3: Confirm
	summaryBox := boxStyle.Render(fmt.Sprintf(
		"📋 Transcription Summary\n\n"+
			"Images:     %d files (%s)\n"+
			"Model:      %s\n"+
			"Output:     %s\n"+
			"Mode:       %s",
		len(images),
		gemini.FormatSize(totalSize),
		getModelDisplayName(opts.Model),
		opts.OutputDir,
		getOrganizationMode(opts),
	))
	fmt.Println(summaryBox)

	var proceed bool
	confirmSelect := huh.NewConfirm().
		Title("Start transcription?").
		Affirmative("Yes, transcribe!").
		Negative("No, cancel").
		Value(&proceed)

	err = huh.NewForm(huh.NewGroup(confirmSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil || !proceed {
		fmt.Println(infoStyle.Render("Transcription cancelled."))
		return askToContinueTranscribe()
	}

	// Step 4: Run transcription
	return runTranscription(opts)
}

// runTranscription executes the transcription with the given options
func runTranscription(opts TranscribeOptions) bool {
	// Create client
	client, err := gemini.NewClientFromEnv(
		gemini.WithDebug(os.Getenv("CAPYCUT_DEBUG") != ""),
	)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		fmt.Println(infoStyle.Render(gemini.GetAPIKeyHelp()))
		return askToContinueTranscribe()
	}

	// Build request
	req := &gemini.TranscribeRequest{
		Images:                   opts.Images,
		OutputDir:                opts.OutputDir,
		Model:                    opts.Model,
		DetectChapters:           opts.DetectChapters,
		CombinePages:             opts.CombinePages,
		PreserveFormatting:       opts.PreserveFormatting,
		IncludeImageDescriptions: opts.IncludeImageDescriptions,
	}

	// Run transcription with spinner
	var resp *gemini.TranscribeResponse
	var transcribeErr error

	startTime := time.Now()

	err = spinner.New().
		Title(fmt.Sprintf("🔍 Transcribing %d images with Gemini...", len(opts.Images))).
		Action(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			resp, transcribeErr = client.TranscribeWithRetry(ctx, req, 3)
		}).
		Run()

	if transcribeErr != nil {
		fmt.Println(errorStyle.Render("Transcription failed: " + transcribeErr.Error()))
		return askToContinueTranscribe()
	}

	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinueTranscribe()
	}

	// Write documents
	fmt.Println(infoStyle.Render("\n📝 Writing markdown files..."))

	writeResult, err := gemini.WriteDocuments(resp.Documents, gemini.WriteOptions{
		OutputDir:          opts.OutputDir,
		Overwrite:          opts.Overwrite,
		AddFrontMatter:     opts.AddFrontMatter,
		AddTableOfContents: opts.AddTableOfContents,
		CreateIndexFile:    opts.CreateIndexFile,
		Verbose:            true,
	})

	if err != nil {
		fmt.Println(errorStyle.Render("Error writing files: " + err.Error()))
		return askToContinueTranscribe()
	}

	// Report errors if any
	if len(writeResult.Errors) > 0 {
		fmt.Println(errorStyle.Render("\n⚠️  Some files could not be written:"))
		for _, err := range writeResult.Errors {
			fmt.Println(errorStyle.Render("  • " + err.Error()))
		}
	}

	// Success summary
	elapsed := time.Since(startTime)
	successBox := boxStyle.Render(fmt.Sprintf(
		"✅ Transcription Complete!\n\n"+
			"📄 Documents created: %d\n"+
			"📦 Total size: %s\n"+
			"⏱️  Time: %s\n"+
			"🎯 Tokens used: %d\n\n"+
			"📂 Output: %s",
		len(writeResult.FilesWritten),
		gemini.FormatSize(writeResult.TotalBytes),
		formatDuration(elapsed),
		resp.TokensUsed,
		opts.OutputDir,
	))
	fmt.Println(successStyle.Render(successBox))

	// List created files
	fmt.Println(infoStyle.Render("\nCreated files:"))
	for _, path := range writeResult.FilesWritten {
		fmt.Println(infoStyle.Render("  • " + filepath.Base(path)))
	}

	return askToContinueTranscribe()
}

// runNonInteractiveTranscribe runs transcription in non-interactive mode
func runNonInteractiveTranscribe(sources []string, outputDir, model string, detectChapters, combine bool) {
	// Load images
	fmt.Println(infoStyle.Render("Loading images..."))
	images, err := gemini.LoadImages(sources)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		os.Exit(1)
	}

	totalSize, count, _ := gemini.GetImageStats(images)
	fmt.Println(infoStyle.Render(fmt.Sprintf("Found %d images (%s)", count, gemini.FormatSize(totalSize))))

	// Create client
	client, err := gemini.NewClientFromEnv(
		gemini.WithDebug(os.Getenv("CAPYCUT_DEBUG") != ""),
	)
	if err != nil {
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		fmt.Println(infoStyle.Render(gemini.GetAPIKeyHelp()))
		os.Exit(1)
	}

	// Set defaults
	if model == "" {
		model = gemini.ModelGemini3Pro
	}
	if outputDir == "" {
		outputDir = "./output"
	}

	// Build request
	req := &gemini.TranscribeRequest{
		Images:             images,
		OutputDir:          outputDir,
		Model:              model,
		DetectChapters:     detectChapters,
		CombinePages:       combine,
		PreserveFormatting: true,
	}

	// Run transcription
	fmt.Println(infoStyle.Render(fmt.Sprintf("Transcribing %d images with %s...", len(images), getModelDisplayName(model))))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	startTime := time.Now()
	resp, err := client.TranscribeWithRetry(ctx, req, 3)
	if err != nil {
		fmt.Println(errorStyle.Render("Transcription failed: " + err.Error()))
		os.Exit(1)
	}

	// Write documents
	writeResult, err := gemini.WriteDocuments(resp.Documents, gemini.WriteOptions{
		OutputDir:       outputDir,
		Overwrite:       true,
		CreateIndexFile: len(resp.Documents) > 1,
	})

	if err != nil {
		fmt.Println(errorStyle.Render("Error writing files: " + err.Error()))
		os.Exit(1)
	}

	// Success
	elapsed := time.Since(startTime)
	fmt.Println(successStyle.Render(fmt.Sprintf(
		"\n✅ Done! Created %d files in %s (took %s, %d tokens)",
		len(writeResult.FilesWritten),
		outputDir,
		formatDuration(elapsed),
		resp.TokensUsed,
	)))

	for _, path := range writeResult.FilesWritten {
		fmt.Println(infoStyle.Render("  • " + path))
	}
}

// Helper functions

func askToContinueTranscribe() bool {
	var choice string
	selectNext := huh.NewSelect[string]().
		Title("What next?").
		Options(
			huh.NewOption("Transcribe more images", "another"),
			huh.NewOption("Back to main menu", "main"),
			huh.NewOption("Exit", "exit"),
		).
		Value(&choice)

	err := huh.NewForm(huh.NewGroup(selectNext)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		return false
	}

	switch choice {
	case "another":
		return runTranscribeWorkflow()
	case "main":
		return true // Return to main menu
	default:
		return false
	}
}

func getModelDisplayName(model string) string {
	switch model {
	case gemini.ModelGemini3Pro:
		return "Gemini 3 Pro"
	case gemini.ModelGemini3ProThinking:
		return "Gemini 3 Pro Thinking"
	case gemini.ModelGemini25Flash:
		return "Gemini 2.5 Flash"
	case gemini.ModelGemini25Pro:
		return "Gemini 2.5 Pro"
	case gemini.ModelGemini20Flash:
		return "Gemini 2.0 Flash"
	default:
		return model
	}
}

func getOrganizationMode(opts TranscribeOptions) string {
	if opts.DetectChapters {
		return "Auto-detect chapters"
	}
	if opts.CombinePages {
		return "Single combined file"
	}
	return "One file per page"
}

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

// checkTranscribeConfig verifies Gemini is configured
func checkTranscribeConfig() error {
	return gemini.CheckConfig()
}

// printTranscribeHelp prints help for the transcribe command
func printTranscribeHelp() {
	help := `
📸 Image to Markdown Transcription

USAGE:
    capycut transcribe [OPTIONS] <images...>

ARGUMENTS:
    <images...>             Image files, directories, or glob patterns
                            Examples:
                              ./scans/*.png
                              /path/to/images/
                              page1.jpg page2.jpg page3.jpg

OPTIONS:
    -o, --output <dir>      Output directory (default: ./output)
    -m, --model <name>      Gemini model to use:
                              3pro     - Gemini 3 Pro (default, most capable)
                              3think   - Gemini 3 Pro Thinking (enhanced reasoning)
                              flash    - Gemini 2.5 Flash (fast)
                              pro      - Gemini 2.5 Pro (previous gen)

    --chapters              Auto-detect and split by chapters
    --combine               Combine all pages into single file
    --language <code>       Document language (auto-detect if not set)

    --debug                 Enable debug output

ENVIRONMENT:
    GEMINI_API_KEY          Your Google Gemini API key
    GOOGLE_API_KEY          Alternative API key variable

EXAMPLES:
    # Transcribe all PNGs in a folder
    capycut transcribe ./scanned_pages/

    # Transcribe with chapter detection
    capycut transcribe --chapters -o ./book/ ./pages/*.png

    # Combine pages into single file
    capycut transcribe --combine -o ./output/ page*.jpg

    # Use Pro model for complex documents
    capycut transcribe -m pro --chapters ./document/

For more info: https://github.com/harmonyvt/capycut
`
	fmt.Println(help)
}

// parseTranscribeArgs parses transcribe command arguments
func parseTranscribeArgs(args []string) (*TranscribeOptions, []string) {
	opts := &TranscribeOptions{}
	var sources []string

	i := 0
	for i < len(args) {
		arg := args[i]

		switch arg {
		case "-o", "--output":
			if i+1 < len(args) {
				opts.OutputDir = args[i+1]
				i += 2
			} else {
				i++
			}
		case "-m", "--model":
			if i+1 < len(args) {
				switch args[i+1] {
				case "3pro", "3-pro", "gemini-3-pro":
					opts.Model = gemini.ModelGemini3Pro
				case "3think", "3-think", "thinking":
					opts.Model = gemini.ModelGemini3ProThinking
				case "flash", "2.5-flash":
					opts.Model = gemini.ModelGemini25Flash
				case "pro", "2.5-pro":
					opts.Model = gemini.ModelGemini25Pro
				case "flash20", "2.0-flash":
					opts.Model = gemini.ModelGemini20Flash
				default:
					opts.Model = args[i+1]
				}
				i += 2
			} else {
				i++
			}
		case "--language":
			if i+1 < len(args) {
				opts.Language = args[i+1]
				i += 2
			} else {
				i++
			}
		case "--chapters":
			opts.DetectChapters = true
			i++
		case "--combine":
			opts.CombinePages = true
			i++
		case "--formatting":
			opts.PreserveFormatting = true
			i++
		case "--images":
			opts.IncludeImageDescriptions = true
			i++
		case "--frontmatter":
			opts.AddFrontMatter = true
			i++
		case "--toc":
			opts.AddTableOfContents = true
			i++
		case "--index":
			opts.CreateIndexFile = true
			i++
		case "--overwrite":
			opts.Overwrite = true
			i++
		case "--help", "-h":
			printTranscribeHelp()
			os.Exit(0)
		default:
			if !strings.HasPrefix(arg, "-") {
				sources = append(sources, arg)
			}
			i++
		}
	}

	return opts, sources
}
