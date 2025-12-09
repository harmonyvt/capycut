package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"capycut/gemini"
	"capycut/tui"

	"github.com/charmbracelet/bubbles/table"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/huh/spinner"
	"github.com/charmbracelet/lipgloss"
)

// TranscribeOptions holds the configuration for image transcription
type TranscribeOptions struct {
	Images                   []string
	OutputDir                string
	Provider                 gemini.Provider // AI provider to use (gemini, local, azure_anthropic)
	Model                    string
	TextModel                string // For two-stage pipeline: text/agentic model for refinement
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

// ============================================================================
// Table-based File Browser
// ============================================================================

// fileEntry represents a file or directory
type fileEntry struct {
	name    string
	path    string
	isDir   bool
	size    int64
	modTime time.Time
}

// fileBrowserModel is the Bubble Tea model for the file browser
type fileBrowserModel struct {
	table      table.Model
	files      []fileEntry
	currentDir string
	selected   string
	quitting   bool
	width      int
	height     int
}

// newFileBrowser creates a new file browser starting at the given directory
func newFileBrowser(startDir string) fileBrowserModel {
	columns := []table.Column{
		{Title: "Type", Width: 4},
		{Title: "Name", Width: 50},
		{Title: "Size", Width: 10},
		{Title: "Modified", Width: 16},
	}

	t := table.New(
		table.WithColumns(columns),
		table.WithFocused(true),
		table.WithHeight(15),
	)

	// Style the table
	s := table.DefaultStyles()
	s.Header = s.Header.
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(lipgloss.Color("240")).
		BorderBottom(true).
		Bold(true).
		Foreground(lipgloss.Color("#7C3AED"))
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(lipgloss.Color("#7C3AED")).
		Bold(true)
	t.SetStyles(s)

	m := fileBrowserModel{
		table:      t,
		currentDir: startDir,
		width:      80,
		height:     24,
	}

	m.loadDirectory(startDir)
	return m
}

// loadDirectory reads directory contents
func (m *fileBrowserModel) loadDirectory(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}

	m.files = []fileEntry{}

	// Add parent directory if not at root
	if dir != "/" {
		m.files = append(m.files, fileEntry{
			name:  "..",
			path:  filepath.Dir(dir),
			isDir: true,
		})
	}

	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".") {
			continue // Skip hidden files
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		m.files = append(m.files, fileEntry{
			name:    entry.Name(),
			path:    filepath.Join(dir, entry.Name()),
			isDir:   entry.IsDir(),
			size:    info.Size(),
			modTime: info.ModTime(),
		})
	}

	// Sort: directories first, then alphabetically
	sort.Slice(m.files, func(i, j int) bool {
		if m.files[i].name == ".." {
			return true
		}
		if m.files[j].name == ".." {
			return false
		}
		if m.files[i].isDir != m.files[j].isDir {
			return m.files[i].isDir
		}
		return strings.ToLower(m.files[i].name) < strings.ToLower(m.files[j].name)
	})

	m.currentDir = dir
	m.updateTableRows()
}

// updateTableRows updates the table with current files
func (m *fileBrowserModel) updateTableRows() {
	rows := make([]table.Row, len(m.files))
	for i, f := range m.files {
		typeIcon := "FILE"
		if f.isDir {
			typeIcon = "DIR"
		}
		if f.name == ".." {
			typeIcon = "UP"
		}

		sizeStr := ""
		if !f.isDir {
			sizeStr = formatSize(f.size)
		}

		modStr := ""
		if f.name != ".." && !f.modTime.IsZero() {
			modStr = f.modTime.Format("Jan 02 15:04")
		}

		rows[i] = table.Row{typeIcon, f.name, sizeStr, modStr}
	}
	m.table.SetRows(rows)
}

func formatSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func (m fileBrowserModel) Init() tea.Cmd {
	return nil
}

func (m fileBrowserModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height

		// Adjust table size
		availableHeight := m.height - 10
		if availableHeight < 5 {
			availableHeight = 5
		}
		if availableHeight > 25 {
			availableHeight = 25
		}
		m.table.SetHeight(availableHeight)

		// Adjust column widths
		nameWidth := m.width - 40
		if nameWidth < 20 {
			nameWidth = 20
		}
		if nameWidth > 80 {
			nameWidth = 80
		}
		m.table.SetColumns([]table.Column{
			{Title: "Type", Width: 4},
			{Title: "Name", Width: nameWidth},
			{Title: "Size", Width: 10},
			{Title: "Modified", Width: 16},
		})

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "esc":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			idx := m.table.Cursor()
			if idx >= 0 && idx < len(m.files) {
				selected := m.files[idx]
				if selected.isDir {
					m.loadDirectory(selected.path)
				}
			}
		case " ", "s":
			// Select current directory
			m.selected = m.currentDir
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.table, cmd = m.table.Update(msg)
	return m, cmd
}

func (m fileBrowserModel) View() string {
	if m.quitting && m.selected == "" {
		return ""
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#7C3AED"))
	mutedStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#64748B"))
	pathStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("#0EA5E9")).Bold(true)

	title := titleStyle.Render("Select a folder containing images")
	desc := mutedStyle.Render("ENTER=open folder | SPACE/s=select this folder | ESC=cancel")

	// Breadcrumb
	breadcrumb := mutedStyle.Render("Location: ") + pathStyle.Render(m.currentDir)
	fileCount := mutedStyle.Render(fmt.Sprintf(" (%d items)", len(m.files)))

	// Box style
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#334155")).
		Padding(1, 2).
		Width(m.width - 4)

	content := title + "\n" +
		desc + "\n\n" +
		breadcrumb + fileCount + "\n\n" +
		m.table.View()

	return boxStyle.Render(content)
}

// runFileBrowser runs the file browser and returns the selected path
func runFileBrowser(startDir string) (string, error) {
	m := newFileBrowser(startDir)
	p := tea.NewProgram(m, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return "", err
	}

	fm := finalModel.(fileBrowserModel)
	if fm.quitting && fm.selected == "" {
		return "", fmt.Errorf("cancelled")
	}
	return fm.selected, nil
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
// Uses the new Bubble Tea TUI by default. Set CAPYCUT_LEGACY_UI=1 to use the old UI.
func runTranscribeWorkflow() bool {
	// Use the new TUI by default, unless user explicitly wants the legacy UI
	if os.Getenv("CAPYCUT_LEGACY_UI") != "1" {
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
		// Use custom table-based folder browser
		startDir, _ := os.Getwd()

		folderPath, err := runFileBrowser(startDir)
		if err != nil {
			if err.Error() == "cancelled" {
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

	// Provider selection - let user choose which AI provider to use
	var providerChoice string
	availableProviders := getAvailableProviders()

	// Default to auto-detected provider
	defaultProvider := string(gemini.GetProvider())

	providerSelect := huh.NewSelect[string]().
		Title("Select AI Provider").
		Description("Choose which AI backend to use for transcription").
		Options(availableProviders...).
		Value(&providerChoice)

	// Set default value
	for _, opt := range availableProviders {
		if opt.Value == defaultProvider {
			providerChoice = defaultProvider
			break
		}
	}

	// First form: output dir and provider selection
	err = huh.NewForm(huh.NewGroup(outputInput, providerSelect)).
		WithTheme(huh.ThemeCatppuccin()).
		Run()

	if err != nil {
		if err == huh.ErrUserAborted {
			return askToContinueTranscribe()
		}
		fmt.Println(errorStyle.Render("Error: " + err.Error()))
		return askToContinueTranscribe()
	}

	// Set the provider
	opts.Provider = gemini.Provider(providerChoice)

	// Model selection - show different options based on selected provider
	var modelChoice string
	var textModelChoice string
	var modelSelect *huh.Select[string]
	var textModelSelect *huh.Select[string]

	if opts.Provider == gemini.ProviderLocal {
		// For local LLM, let user specify model or use default from env
		defaultModel := os.Getenv("LLM_MODEL")
		if defaultModel == "" {
			defaultModel = "local-model"
		}
		modelSelect = huh.NewSelect[string]().
			Title("Select vision model").
			Description("Vision model for image scanning (must support images)").
			Options(
				huh.NewOption(fmt.Sprintf("Default (%s)", defaultModel), defaultModel),
				huh.NewOption("llava (LLaVA vision model)", "llava"),
				huh.NewOption("llava:13b (LLaVA 13B)", "llava:13b"),
				huh.NewOption("bakllava (BakLLaVA)", "bakllava"),
				huh.NewOption("qwen-vl (Qwen Vision)", "qwen-vl"),
				huh.NewOption("minicpm-v (MiniCPM-V)", "minicpm-v"),
			).
			Value(&modelChoice)

		// Text model selection for two-stage pipeline
		defaultTextModel := os.Getenv("IMAGE_TEXT_MODEL")
		textModelSelect = huh.NewSelect[string]().
			Title("Select text model (optional)").
			Description("Agentic model to refine extracted text into polished markdown").
			Options(
				huh.NewOption("None (single-stage, vision model does everything)", ""),
				huh.NewOption(fmt.Sprintf("From env: %s", defaultTextModel), defaultTextModel).Selected(defaultTextModel != ""),
				huh.NewOption("mistral (Mistral 7B)", "mistral"),
				huh.NewOption("llama3.2 (Llama 3.2)", "llama3.2"),
				huh.NewOption("llama3.1 (Llama 3.1)", "llama3.1"),
				huh.NewOption("qwen2.5 (Qwen 2.5)", "qwen2.5"),
				huh.NewOption("gemma2 (Gemma 2)", "gemma2"),
				huh.NewOption("phi3 (Phi-3)", "phi3"),
				huh.NewOption("deepseek-r1 (DeepSeek R1)", "deepseek-r1"),
			).
			Value(&textModelChoice)
	} else if opts.Provider == gemini.ProviderAzureAnthropic {
		// For Azure Anthropic (Claude), show Claude model options
		envModel := os.Getenv("AZURE_ANTHROPIC_MODEL")

		// Build options list
		claudeOptions := []huh.Option[string]{}

		// Add env model first if configured
		if envModel != "" {
			claudeOptions = append(claudeOptions,
				huh.NewOption(fmt.Sprintf("From env: %s", envModel), envModel),
			)
		}

		// Add standard Claude models with raw IDs shown
		claudeOptions = append(claudeOptions,
			huh.NewOption("Claude Sonnet 4.5 (claude-sonnet-4-5-20250514)", "claude-sonnet-4-5-20250514"),
			huh.NewOption("Claude Sonnet 4 (claude-sonnet-4-20250514)", "claude-sonnet-4-20250514"),
			huh.NewOption("Claude 3.5 Sonnet (claude-3-5-sonnet-20241022)", "claude-3-5-sonnet-20241022"),
			huh.NewOption("Claude 3 Opus (claude-3-opus-20240229)", "claude-3-opus-20240229"),
			huh.NewOption("Claude 3 Sonnet (claude-3-sonnet-20240229)", "claude-3-sonnet-20240229"),
			huh.NewOption("Claude 3 Haiku (claude-3-haiku-20240307)", "claude-3-haiku-20240307"),
		)

		modelSelect = huh.NewSelect[string]().
			Title("Select Claude model").
			Description("Choose the Claude model to use for transcription").
			Options(claudeOptions...).
			Value(&modelChoice)
		// No text model selection for Claude (single powerful model)
		textModelSelect = nil
	} else {
		// Default: Gemini provider
		modelSelect = huh.NewSelect[string]().
			Title("Select Gemini model").
			Options(
				huh.NewOption("Gemini 3 Pro (Latest, most capable)", gemini.ModelGemini3Pro),
				huh.NewOption("Gemini 2.5 Flash (Fast)", gemini.ModelGemini25Flash),
			).
			Value(&modelChoice)
		// No text model selection for Gemini (single powerful model)
		textModelSelect = nil
	}

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

	// Build form groups - add text model selection for local LLM
	var formGroups []*huh.Group
	if textModelSelect != nil {
		formGroups = []*huh.Group{
			huh.NewGroup(outputInput, modelSelect, textModelSelect),
			huh.NewGroup(orgSelect, additionalSelect),
		}
	} else {
		formGroups = []*huh.Group{
			huh.NewGroup(outputInput, modelSelect),
			huh.NewGroup(orgSelect, additionalSelect),
		}
	}

	err = huh.NewForm(formGroups...).WithTheme(huh.ThemeCatppuccin()).Run()

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
	opts.TextModel = textModelChoice

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
	providerInfo := getProviderDisplayNameForProvider(opts.Provider)

	// Build model info string
	modelInfo := getModelDisplayName(opts.Model)
	if opts.TextModel != "" {
		modelInfo = fmt.Sprintf("%s (vision) + %s (text)", opts.Model, opts.TextModel)
	}

	summaryBox := boxStyle.Render(fmt.Sprintf(
		"📋 Transcription Summary\n\n"+
			"Images:     %d files (%s)\n"+
			"Provider:   %s\n"+
			"Model:      %s\n"+
			"Output:     %s\n"+
			"Mode:       %s",
		len(images),
		gemini.FormatSize(totalSize),
		providerInfo,
		modelInfo,
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
	// Create client based on selected provider
	var client *gemini.Client
	var err error

	clientOpts := []gemini.ClientOption{
		gemini.WithDebug(os.Getenv("CAPYCUT_DEBUG") != ""),
	}
	// If user selected a text model in UI, override the env var setting
	if opts.TextModel != "" {
		clientOpts = append(clientOpts, gemini.WithTextModel(opts.TextModel))
	}

	switch opts.Provider {
	case gemini.ProviderLocal:
		// Use local LLM
		endpoint := os.Getenv("LLM_ENDPOINT")
		if endpoint == "" {
			endpoint = os.Getenv("IMAGE_LLM_ENDPOINT")
		}
		if endpoint == "" {
			fmt.Println(errorStyle.Render("Error: LLM_ENDPOINT not configured"))
			fmt.Println(infoStyle.Render("Set LLM_ENDPOINT to your local LLM server URL (e.g., http://localhost:1234)"))
			return askToContinueTranscribe()
		}
		client, err = gemini.NewLocalClient(endpoint, opts.Model, clientOpts...)

	case gemini.ProviderAzureAnthropic:
		// Use Azure Anthropic (Claude)
		endpoint := os.Getenv("AZURE_ANTHROPIC_ENDPOINT")
		apiKey := os.Getenv("AZURE_ANTHROPIC_API_KEY")
		if endpoint == "" || apiKey == "" {
			fmt.Println(errorStyle.Render("Error: Azure Anthropic not configured"))
			fmt.Println(infoStyle.Render("Set AZURE_ANTHROPIC_ENDPOINT and AZURE_ANTHROPIC_API_KEY environment variables"))
			return askToContinueTranscribe()
		}
		client, err = gemini.NewAzureAnthropicClient(endpoint, apiKey, opts.Model, clientOpts...)

	default:
		// Use Gemini (default)
		apiKey := os.Getenv("GEMINI_API_KEY")
		if apiKey == "" {
			apiKey = os.Getenv("GOOGLE_API_KEY")
		}
		if apiKey == "" {
			fmt.Println(errorStyle.Render("Error: Gemini API key not configured"))
			fmt.Println(infoStyle.Render(gemini.GetAPIKeyHelp()))
			return askToContinueTranscribe()
		}
		client, err = gemini.NewClient(apiKey, clientOpts...)
	}

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

	// Show AI status box before transcription
	providerName := getProviderDisplayNameForProvider(opts.Provider)
	modelDisplay := opts.Model
	if opts.TextModel != "" {
		modelDisplay = fmt.Sprintf("%s (vision) + %s (text)", opts.Model, opts.TextModel)
	}

	aiStatusBox := boxStyle.Render(fmt.Sprintf(
		"🤖 AI Agent: %s\n"+
			"   Model: %s\n"+
			"   Images: %d\n"+
			"   Status: Starting...",
		providerName,
		modelDisplay,
		len(opts.Images),
	))
	fmt.Println(aiStatusBox)

	// Run transcription with spinner
	var resp *gemini.TranscribeResponse
	var transcribeErr error
	var lastStatus string

	startTime := time.Now()

	// Progress callback for status updates
	onProgress := func(update gemini.ProgressUpdate) {
		newStatus := fmt.Sprintf("[%s] %s", update.Status.String(), update.Message)
		if update.Detail != "" {
			newStatus += " - " + update.Detail
		}
		if newStatus != lastStatus {
			if os.Getenv("CAPYCUT_DEBUG") != "" {
				fmt.Printf("\n   AI Status: %s\n", newStatus)
			}
			lastStatus = newStatus
		}
	}

	// Determine spinner message based on provider and pipeline mode
	spinnerMsg := fmt.Sprintf("🔍 Transcribing %d images with %s...", len(opts.Images), providerName)

	err = spinner.New().
		Title(spinnerMsg).
		Action(func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
			defer cancel()
			resp, transcribeErr = client.TranscribeImagesWithProgress(ctx, req, onProgress)
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

	// Show AI completion
	fmt.Println(successStyle.Render("✓ AI processing complete"))

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

	// Show AI status box
	providerName := getProviderDisplayName()
	aiStatusBox := boxStyle.Render(fmt.Sprintf(
		"🤖 AI Agent: %s\n"+
			"   Model: %s\n"+
			"   Images: %d\n"+
			"   Status: Starting...",
		providerName,
		model,
		len(images),
	))
	fmt.Println(aiStatusBox)

	// Build request
	req := &gemini.TranscribeRequest{
		Images:             images,
		OutputDir:          outputDir,
		Model:              model,
		DetectChapters:     detectChapters,
		CombinePages:       combine,
		PreserveFormatting: true,
	}

	// Progress callback
	onProgress := func(update gemini.ProgressUpdate) {
		statusLine := fmt.Sprintf("   [%s] %s", update.Status.String(), update.Message)
		if update.Detail != "" {
			statusLine += " - " + update.Detail
		}
		fmt.Printf("\r%s", statusLine)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	startTime := time.Now()
	resp, err := client.TranscribeImagesWithProgress(ctx, req, onProgress)
	fmt.Println() // New line after progress updates

	if err != nil {
		fmt.Println(errorStyle.Render("Transcription failed: " + err.Error()))
		os.Exit(1)
	}

	fmt.Println(successStyle.Render("✓ AI processing complete"))

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

func getProviderDisplayName() string {
	return getProviderDisplayNameForProvider(gemini.GetProvider())
}

func getProviderDisplayNameForProvider(provider gemini.Provider) string {
	switch provider {
	case gemini.ProviderGemini:
		return "Google Gemini"
	case gemini.ProviderLocal:
		endpoint := os.Getenv("LLM_ENDPOINT")
		if endpoint != "" {
			return fmt.Sprintf("Local LLM (%s)", endpoint)
		}
		return "Local LLM"
	case gemini.ProviderAzureAnthropic:
		return "Azure Anthropic (Claude)"
	default:
		return string(provider)
	}
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

// getAvailableProviders returns a list of available AI providers based on environment config
func getAvailableProviders() []huh.Option[string] {
	var options []huh.Option[string]

	// Check for Gemini API key
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
		options = append(options, huh.NewOption("Google Gemini (Cloud API)", string(gemini.ProviderGemini)))
	}

	// Check for Local LLM endpoint
	if os.Getenv("LLM_ENDPOINT") != "" || os.Getenv("IMAGE_LLM_ENDPOINT") != "" {
		endpoint := os.Getenv("LLM_ENDPOINT")
		if endpoint == "" {
			endpoint = os.Getenv("IMAGE_LLM_ENDPOINT")
		}
		options = append(options, huh.NewOption(fmt.Sprintf("Local LLM (%s)", endpoint), string(gemini.ProviderLocal)))
	}

	// Check for Azure Anthropic
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" {
		options = append(options, huh.NewOption("Azure Anthropic (Claude)", string(gemini.ProviderAzureAnthropic)))
	}

	// If no providers configured, still show options but mark as unconfigured
	if len(options) == 0 {
		options = append(options,
			huh.NewOption("Google Gemini (not configured)", string(gemini.ProviderGemini)),
			huh.NewOption("Local LLM (not configured)", string(gemini.ProviderLocal)),
			huh.NewOption("Azure Anthropic (not configured)", string(gemini.ProviderAzureAnthropic)),
		)
	}

	return options
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
    -m, --model <name>      Model to use:
                            Gemini (when using GEMINI_API_KEY):
                              3pro     - Gemini 3 Pro (default, most capable)
                              3think   - Gemini 3 Pro Thinking (enhanced reasoning)
                              flash    - Gemini 2.5 Flash (fast)
                              pro      - Gemini 2.5 Pro (previous gen)
                            Local LLM (when using LLM_ENDPOINT):
                              llava    - LLaVA vision model
                              or any model name supported by your local server

    --chapters              Auto-detect and split by chapters
    --combine               Combine all pages into single file
    --language <code>       Document language (auto-detect if not set)

    --debug                 Enable debug output

ENVIRONMENT:
    Option 1: Local LLM (FREE - uses same config as video clipping)
    LLM_ENDPOINT            Local LLM server URL (e.g., http://localhost:1234)
    LLM_MODEL               Model name (e.g., llava, qwen-vl)

    Option 2: Google Gemini API
    GEMINI_API_KEY          Your Google Gemini API key
    GOOGLE_API_KEY          Alternative API key variable

EXAMPLES:
    # Transcribe all PNGs in a folder
    capycut transcribe ./scanned_pages/

    # Transcribe with chapter detection
    capycut transcribe --chapters -o ./book/ ./pages/*.png

    # Combine pages into single file
    capycut transcribe --combine -o ./output/ page*.jpg

    # Use local LLM with LLaVA model
    LLM_ENDPOINT=http://localhost:1234 capycut transcribe ./document/

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
