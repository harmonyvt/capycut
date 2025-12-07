package tui

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"capycut/gemini"

	"github.com/charmbracelet/bubbles/filepicker"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TranscribeStep represents the current step in the transcription workflow
type TranscribeStep int

const (
	TStepSelectSource TranscribeStep = iota
	TStepSelectFolder
	TStepEnterPattern
	TStepLoadingImages
	TStepConfigureOutput
	TStepSelectModel
	TStepSelectOrganization
	TStepSelectOptions
	TStepConfirm
	TStepTranscribing
	TStepWriting
	TStepComplete
	TStepError
)

// TranscribeModel is the Bubble Tea model for the transcription workflow
type TranscribeModel struct {
	// Current step
	step TranscribeStep

	// UI Components
	filepicker filepicker.Model
	textInput  textinput.Model
	spinner    spinner.Model
	progress   progress.Model

	// Selection state
	selectedSource     string // "folder" or "pattern"
	selectedFolder     string
	inputPattern       string
	outputDir          string
	selectedModel      string
	selectedModelIndex int
	orgMode            string // "chapters", "combine", "pages"
	orgModeIndex       int
	options            []bool // formatting, images, frontmatter, toc, index, overwrite
	optionIndex        int

	// Image data
	images     []string
	totalSize  int64
	imageCount int

	// Progress tracking
	transcribeProgress float64
	currentBatch       int
	totalBatches       int
	statusMessage      string

	// Results
	result       *gemini.TranscribeResponse
	writeResult  *gemini.WriteResult
	errorMessage string
	startTime    time.Time

	// Dimensions
	width  int
	height int

	// State flags
	confirmed  bool
	quitting   bool
	backToMenu bool

	// Menu indices
	sourceMenuIndex int
	confirmIndex    int

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// TranscribeResult is sent when transcription completes
type transcribeResultMsg struct {
	response *gemini.TranscribeResponse
	err      error
}

// writeResultMsg is sent when writing completes
type writeResultMsg struct {
	result *gemini.WriteResult
	err    error
}

// imagesLoadedMsg is sent when images are loaded
type imagesLoadedMsg struct {
	images    []string
	totalSize int64
	count     int
	err       error
}

// progressMsg updates the progress
type progressMsg struct {
	progress float64
	batch    int
	total    int
	message  string
}

// fileSelectedMsg is sent when a file/folder is selected
type fileSelectedMsg string

// Model options
var (
	modelOptions = []struct {
		name  string
		value string
		desc  string
	}{
		{"Gemini 3 Pro", gemini.ModelGemini3Pro, "Latest, most capable"},
		{"Gemini 3 Pro Thinking", gemini.ModelGemini3ProThinking, "Enhanced reasoning"},
		{"Gemini 2.5 Flash", gemini.ModelGemini25Flash, "Fast & efficient"},
		{"Gemini 2.5 Pro", gemini.ModelGemini25Pro, "Previous generation"},
	}

	orgOptions = []struct {
		name  string
		value string
		desc  string
	}{
		{"Auto-detect chapters", "chapters", "Split by detected chapter headings"},
		{"Combine into one file", "combine", "Merge all pages into single document"},
		{"One file per page", "pages", "Create separate file for each page"},
	}

	additionalOptions = []struct {
		name string
		desc string
	}{
		{"Preserve formatting", "Tables, lists, code blocks"},
		{"Include image descriptions", "Describe figures and charts"},
		{"Add YAML front matter", "Metadata header"},
		{"Add table of contents", "Auto-generated TOC"},
		{"Create index file", "For multiple documents"},
		{"Overwrite existing", "Replace existing files"},
	}
)

// NewTranscribeModel creates a new transcription model
func NewTranscribeModel() TranscribeModel {
	// Initialize file picker
	fp := filepicker.New()
	fp.DirAllowed = true
	fp.FileAllowed = false
	fp.ShowHidden = false
	fp.ShowSize = true
	fp.Height = 12

	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "./images/*.png"
	ti.CharLimit = 256
	ti.Width = 50

	// Initialize spinner with custom style
	s := spinner.New()
	s.Spinner = spinner.Spinner{
		Frames: []string{"( o.o ) ", "( o.o )>", "( o.o)>>", "(o.o )>>", "( o.o)> ", "( o.o ) "},
		FPS:    time.Second / 8,
	}
	s.Style = lipgloss.NewStyle().Foreground(ColorCapybara)

	// Initialize progress bar
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(50),
	)

	ctx, cancel := context.WithCancel(context.Background())

	return TranscribeModel{
		step:       TStepSelectSource,
		filepicker: fp,
		textInput:  ti,
		spinner:    s,
		progress:   p,
		options:    make([]bool, len(additionalOptions)),
		width:      80,
		height:     24,
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Init initializes the model
func (m TranscribeModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.filepicker.Init(),
	)
}

// Update handles messages
func (m TranscribeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = m.width - 20
		return m, nil

	case tea.KeyMsg:
		// Global key handlers
		switch msg.String() {
		case "ctrl+c", "q":
			if m.step != TStepTranscribing && m.step != TStepWriting {
				m.quitting = true
				m.cancel()
				return m, tea.Quit
			}
		case "esc":
			if m.step == TStepTranscribing || m.step == TStepWriting {
				// Cancel in progress
				m.cancel()
				m.errorMessage = "Cancelled by user"
				m.step = TStepError
				return m, nil
			}
			// Go back a step
			return m.goBack()
		}

		// Step-specific handlers
		return m.handleStepInput(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case progress.FrameMsg:
		progressModel, cmd := m.progress.Update(msg)
		m.progress = progressModel.(progress.Model)
		return m, cmd

	case imagesLoadedMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = TStepError
			return m, nil
		}
		m.images = msg.images
		m.totalSize = msg.totalSize
		m.imageCount = msg.count
		m.step = TStepConfigureOutput
		m.textInput.SetValue("./output")
		m.textInput.Focus()
		return m, nil

	case transcribeResultMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = TStepError
			return m, nil
		}
		m.result = msg.response
		m.step = TStepWriting
		return m, m.writeDocuments()

	case writeResultMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = TStepError
			return m, nil
		}
		m.writeResult = msg.result
		m.step = TStepComplete
		return m, nil

	case progressMsg:
		m.transcribeProgress = msg.progress
		m.currentBatch = msg.batch
		m.totalBatches = msg.total
		m.statusMessage = msg.message
		return m, m.progress.SetPercent(msg.progress)

	case fileSelectedMsg:
		m.selectedFolder = string(msg)
		m.step = TStepLoadingImages
		return m, m.loadImages([]string{string(msg)})
	}

	// Update sub-components based on step
	switch m.step {
	case TStepSelectFolder:
		var cmd tea.Cmd
		m.filepicker, cmd = m.filepicker.Update(msg)

		// Check if a directory was selected
		if didSelect, path := m.filepicker.DidSelectFile(msg); didSelect {
			return m, func() tea.Msg { return fileSelectedMsg(path) }
		}
		if didSelect, path := m.filepicker.DidSelectDisabledFile(msg); didSelect {
			// They selected a directory
			return m, func() tea.Msg { return fileSelectedMsg(path) }
		}
		return m, cmd

	case TStepEnterPattern:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd

	case TStepConfigureOutput:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleStepInput handles keyboard input for specific steps
func (m TranscribeModel) handleStepInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case TStepSelectSource:
		switch msg.String() {
		case "up", "k":
			if m.sourceMenuIndex > 0 {
				m.sourceMenuIndex--
			}
		case "down", "j":
			if m.sourceMenuIndex < 1 {
				m.sourceMenuIndex++
			}
		case "enter":
			if m.sourceMenuIndex == 0 {
				m.selectedSource = "folder"
				m.step = TStepSelectFolder
				return m, m.filepicker.Init()
			} else {
				m.selectedSource = "pattern"
				m.step = TStepEnterPattern
				m.textInput.Placeholder = "./images/*.png or /path/to/folder"
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}

	case TStepEnterPattern:
		switch msg.String() {
		case "enter":
			pattern := m.textInput.Value()
			if pattern != "" {
				m.inputPattern = pattern
				m.step = TStepLoadingImages
				return m, m.loadImages([]string{pattern})
			}
		}

	case TStepConfigureOutput:
		switch msg.String() {
		case "enter":
			m.outputDir = m.textInput.Value()
			if m.outputDir == "" {
				m.outputDir = "./output"
			}
			m.step = TStepSelectModel
		}

	case TStepSelectModel:
		switch msg.String() {
		case "up", "k":
			if m.selectedModelIndex > 0 {
				m.selectedModelIndex--
			}
		case "down", "j":
			if m.selectedModelIndex < len(modelOptions)-1 {
				m.selectedModelIndex++
			}
		case "enter":
			m.selectedModel = modelOptions[m.selectedModelIndex].value
			m.step = TStepSelectOrganization
		}

	case TStepSelectOrganization:
		switch msg.String() {
		case "up", "k":
			if m.orgModeIndex > 0 {
				m.orgModeIndex--
			}
		case "down", "j":
			if m.orgModeIndex < len(orgOptions)-1 {
				m.orgModeIndex++
			}
		case "enter":
			m.orgMode = orgOptions[m.orgModeIndex].value
			m.step = TStepSelectOptions
		}

	case TStepSelectOptions:
		switch msg.String() {
		case "up", "k":
			if m.optionIndex > 0 {
				m.optionIndex--
			}
		case "down", "j":
			if m.optionIndex < len(additionalOptions)-1 {
				m.optionIndex++
			}
		case " ":
			m.options[m.optionIndex] = !m.options[m.optionIndex]
		case "enter":
			m.step = TStepConfirm
		}

	case TStepConfirm:
		switch msg.String() {
		case "up", "k", "left", "h":
			if m.confirmIndex > 0 {
				m.confirmIndex--
			}
		case "down", "j", "right", "l":
			if m.confirmIndex < 1 {
				m.confirmIndex++
			}
		case "enter":
			if m.confirmIndex == 0 {
				// Yes, start transcription
				m.confirmed = true
				m.step = TStepTranscribing
				m.startTime = time.Now()
				return m, m.startTranscription()
			} else {
				// Cancel
				m.backToMenu = true
				return m, tea.Quit
			}
		case "y", "Y":
			m.confirmed = true
			m.step = TStepTranscribing
			m.startTime = time.Now()
			return m, m.startTranscription()
		case "n", "N":
			m.backToMenu = true
			return m, tea.Quit
		}

	case TStepComplete:
		switch msg.String() {
		case "enter", "q":
			return m, tea.Quit
		case "a":
			// Start another transcription
			newModel := NewTranscribeModel()
			return newModel, newModel.Init()
		}

	case TStepError:
		switch msg.String() {
		case "enter", "r":
			// Retry from beginning
			newModel := NewTranscribeModel()
			return newModel, newModel.Init()
		case "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

// goBack navigates to the previous step
func (m TranscribeModel) goBack() (tea.Model, tea.Cmd) {
	switch m.step {
	case TStepSelectFolder, TStepEnterPattern:
		m.step = TStepSelectSource
	case TStepConfigureOutput:
		if m.selectedSource == "folder" {
			m.step = TStepSelectFolder
		} else {
			m.step = TStepEnterPattern
		}
	case TStepSelectModel:
		m.step = TStepConfigureOutput
		m.textInput.SetValue(m.outputDir)
		m.textInput.Focus()
	case TStepSelectOrganization:
		m.step = TStepSelectModel
	case TStepSelectOptions:
		m.step = TStepSelectOrganization
	case TStepConfirm:
		m.step = TStepSelectOptions
	}
	return m, nil
}

// loadImages loads images from the specified sources
func (m TranscribeModel) loadImages(sources []string) tea.Cmd {
	return func() tea.Msg {
		images, err := gemini.LoadImages(sources)
		if err != nil {
			return imagesLoadedMsg{err: err}
		}
		totalSize, count, _ := gemini.GetImageStats(images)
		return imagesLoadedMsg{
			images:    images,
			totalSize: totalSize,
			count:     count,
		}
	}
}

// startTranscription begins the transcription process
func (m TranscribeModel) startTranscription() tea.Cmd {
	return func() tea.Msg {
		client, err := gemini.NewClientFromEnv()
		if err != nil {
			return transcribeResultMsg{err: err}
		}

		req := &gemini.TranscribeRequest{
			Images:                   m.images,
			OutputDir:                m.outputDir,
			Model:                    m.selectedModel,
			DetectChapters:           m.orgMode == "chapters",
			CombinePages:             m.orgMode == "combine",
			PreserveFormatting:       m.options[0],
			IncludeImageDescriptions: m.options[1],
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		resp, err := client.TranscribeWithRetry(ctx, req, 3)
		return transcribeResultMsg{
			response: resp,
			err:      err,
		}
	}
}

// writeDocuments writes the transcription results
func (m TranscribeModel) writeDocuments() tea.Cmd {
	return func() tea.Msg {
		if m.result == nil {
			return writeResultMsg{err: fmt.Errorf("no transcription result")}
		}

		result, err := gemini.WriteDocuments(m.result.Documents, gemini.WriteOptions{
			OutputDir:          m.outputDir,
			Overwrite:          m.options[5],
			AddFrontMatter:     m.options[2],
			AddTableOfContents: m.options[3],
			CreateIndexFile:    m.options[4],
		})

		return writeResultMsg{
			result: result,
			err:    err,
		}
	}
}

// View renders the UI
func (m TranscribeModel) View() string {
	if m.quitting {
		return MutedStyle.Render("Goodbye!\n")
	}

	var b strings.Builder

	// Header
	b.WriteString(GetCapybaraHeader())
	b.WriteString("\n")

	// Step indicator
	b.WriteString(m.renderStepIndicator())
	b.WriteString("\n")

	// Main content based on step
	switch m.step {
	case TStepSelectSource:
		b.WriteString(m.renderSourceSelection())
	case TStepSelectFolder:
		b.WriteString(m.renderFolderPicker())
	case TStepEnterPattern:
		b.WriteString(m.renderPatternInput())
	case TStepLoadingImages:
		b.WriteString(m.renderLoading("Loading images..."))
	case TStepConfigureOutput:
		b.WriteString(m.renderOutputConfig())
	case TStepSelectModel:
		b.WriteString(m.renderModelSelection())
	case TStepSelectOrganization:
		b.WriteString(m.renderOrgSelection())
	case TStepSelectOptions:
		b.WriteString(m.renderOptionsSelection())
	case TStepConfirm:
		b.WriteString(m.renderConfirmation())
	case TStepTranscribing:
		b.WriteString(m.renderTranscribing())
	case TStepWriting:
		b.WriteString(m.renderWriting())
	case TStepComplete:
		b.WriteString(m.renderComplete())
	case TStepError:
		b.WriteString(m.renderError())
	}

	// Help footer
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderStepIndicator shows the current progress through the wizard
func (m TranscribeModel) renderStepIndicator() string {
	steps := []struct {
		name   string
		active bool
		done   bool
	}{
		{"Source", m.step >= TStepSelectSource, m.step > TStepLoadingImages},
		{"Output", m.step >= TStepConfigureOutput, m.step > TStepConfigureOutput},
		{"Model", m.step >= TStepSelectModel, m.step > TStepSelectModel},
		{"Options", m.step >= TStepSelectOrganization, m.step > TStepSelectOptions},
		{"Confirm", m.step >= TStepConfirm, m.step > TStepConfirm},
		{"Process", m.step >= TStepTranscribing, m.step >= TStepComplete},
	}

	var parts []string
	for i, s := range steps {
		var style lipgloss.Style
		var icon string

		if s.done {
			icon = "[x]"
			style = lipgloss.NewStyle().Foreground(ColorSuccess)
		} else if s.active {
			icon = "[>]"
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		} else {
			icon = "[ ]"
			style = lipgloss.NewStyle().Foreground(ColorMuted)
		}

		stepStr := style.Render(icon + " " + s.name)
		parts = append(parts, stepStr)

		if i < len(steps)-1 {
			connector := "---"
			if s.done {
				parts = append(parts, lipgloss.NewStyle().Foreground(ColorSuccess).Render(connector))
			} else {
				parts = append(parts, lipgloss.NewStyle().Foreground(ColorBorder).Render(connector))
			}
		}
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, parts...)
}

// renderSourceSelection renders the source selection menu
func (m TranscribeModel) renderSourceSelection() string {
	title := TitleStyle.Render("How would you like to select images?")

	options := []string{
		"Select a folder containing images",
		"Enter file paths or glob pattern",
	}

	var items strings.Builder
	for i, opt := range options {
		cursor := "  "
		style := BodyStyle
		if i == m.sourceMenuIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		items.WriteString(style.Render(cursor+opt) + "\n")
	}

	return BoxStyle.Render(title + "\n\n" + items.String())
}

// renderFolderPicker renders the folder picker
func (m TranscribeModel) renderFolderPicker() string {
	title := TitleStyle.Render("Select a folder containing images")
	desc := MutedStyle.Render("Navigate and select a folder with your images (JPG, PNG, etc.)")

	return BoxStyle.Render(title + "\n" + desc + "\n\n" + m.filepicker.View())
}

// renderPatternInput renders the pattern input
func (m TranscribeModel) renderPatternInput() string {
	title := TitleStyle.Render("Enter file paths or glob pattern")
	desc := MutedStyle.Render("Examples: /path/to/images/*.png, ./scans/page_*.jpg")

	return BoxStyle.Render(title + "\n" + desc + "\n\n" + m.textInput.View())
}

// renderOutputConfig renders output configuration
func (m TranscribeModel) renderOutputConfig() string {
	title := TitleStyle.Render("Output Configuration")

	// Show image stats
	stats := fmt.Sprintf("Found %d images (%.2f MB)",
		m.imageCount,
		float64(m.totalSize)/(1024*1024))
	statsStyle := InfoStyle.Render(stats)

	// First and last image names
	var imageInfo string
	if len(m.images) > 0 {
		imageInfo = MutedStyle.Render(fmt.Sprintf("First: %s\nLast:  %s",
			filepath.Base(m.images[0]),
			filepath.Base(m.images[len(m.images)-1])))
	}

	inputLabel := BodyStyle.Render("Output directory:")

	return BoxStyle.Render(
		title + "\n\n" +
			statsStyle + "\n" +
			imageInfo + "\n\n" +
			inputLabel + "\n" +
			m.textInput.View(),
	)
}

// renderModelSelection renders the model selection
func (m TranscribeModel) renderModelSelection() string {
	title := TitleStyle.Render("Select Gemini Model")

	var items strings.Builder
	for i, opt := range modelOptions {
		cursor := "  "
		style := BodyStyle
		if i == m.selectedModelIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		items.WriteString(style.Render(cursor+opt.name) +
			MutedStyle.Render(" - "+opt.desc) + "\n")
	}

	return BoxStyle.Render(title + "\n\n" + items.String())
}

// renderOrgSelection renders organization selection
func (m TranscribeModel) renderOrgSelection() string {
	title := TitleStyle.Render("Document Organization")

	var items strings.Builder
	for i, opt := range orgOptions {
		cursor := "  "
		style := BodyStyle
		if i == m.orgModeIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		items.WriteString(style.Render(cursor+opt.name) +
			MutedStyle.Render(" - "+opt.desc) + "\n")
	}

	return BoxStyle.Render(title + "\n\n" + items.String())
}

// renderOptionsSelection renders additional options
func (m TranscribeModel) renderOptionsSelection() string {
	title := TitleStyle.Render("Additional Options")
	hint := MutedStyle.Render("Space to toggle, Enter to continue")

	var items strings.Builder
	for i, opt := range additionalOptions {
		cursor := "  "
		style := BodyStyle
		if i == m.optionIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}

		checkbox := "[ ]"
		if m.options[i] {
			checkbox = "[x]"
		}

		items.WriteString(style.Render(cursor+checkbox+" "+opt.name) +
			MutedStyle.Render(" - "+opt.desc) + "\n")
	}

	return BoxStyle.Render(title + "\n" + hint + "\n\n" + items.String())
}

// renderConfirmation renders the confirmation screen
func (m TranscribeModel) renderConfirmation() string {
	title := TitleStyle.Render("Ready to Transcribe!")

	// Summary
	summary := fmt.Sprintf(`Images:     %d files (%.2f MB)
Model:      %s
Output:     %s
Mode:       %s`,
		m.imageCount,
		float64(m.totalSize)/(1024*1024),
		modelOptions[m.selectedModelIndex].name,
		m.outputDir,
		orgOptions[m.orgModeIndex].name,
	)

	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSecondary).
		Padding(1, 2).
		Render(summary)

	// Buttons
	yesStyle := BodyStyle
	noStyle := BodyStyle
	if m.confirmIndex == 0 {
		yesStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorSuccess).
			Padding(0, 2)
	} else {
		yesStyle = lipgloss.NewStyle().
			Foreground(ColorSuccess).
			Padding(0, 2)
	}
	if m.confirmIndex == 1 {
		noStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FFFFFF")).
			Background(ColorError).
			Padding(0, 2)
	} else {
		noStyle = lipgloss.NewStyle().
			Foreground(ColorError).
			Padding(0, 2)
	}

	buttons := lipgloss.JoinHorizontal(
		lipgloss.Center,
		yesStyle.Render("Yes, transcribe!"),
		"  ",
		noStyle.Render("Cancel"),
	)

	return BoxStyle.Render(title + "\n\n" + summaryBox + "\n\n" + buttons)
}

// renderLoading renders a loading state
func (m TranscribeModel) renderLoading(message string) string {
	return BoxStyle.Render(
		m.spinner.View() + " " + BodyStyle.Render(message),
	)
}

// renderTranscribing renders the transcription progress
func (m TranscribeModel) renderTranscribing() string {
	title := TitleStyle.Render("Transcribing...")

	spinnerView := m.spinner.View()
	status := BodyStyle.Render(fmt.Sprintf("Processing %d images with Gemini...", m.imageCount))

	progressView := m.progress.View()

	elapsed := time.Since(m.startTime)
	elapsedStr := MutedStyle.Render(fmt.Sprintf("Elapsed: %s", formatDuration(elapsed)))

	return BoxStyle.Render(
		title + "\n\n" +
			spinnerView + " " + status + "\n\n" +
			progressView + "\n" +
			elapsedStr,
	)
}

// renderWriting renders the writing progress
func (m TranscribeModel) renderWriting() string {
	title := TitleStyle.Render("Writing Files...")

	spinnerView := m.spinner.View()
	status := BodyStyle.Render("Saving markdown files to disk...")

	return BoxStyle.Render(
		title + "\n\n" +
			spinnerView + " " + status,
	)
}

// renderComplete renders the completion screen
func (m TranscribeModel) renderComplete() string {
	title := SuccessStyle.Render("Transcription Complete!")

	elapsed := time.Since(m.startTime)
	tokensUsed := 0
	docsCreated := 0
	var totalBytes int64

	if m.result != nil {
		tokensUsed = m.result.TokensUsed
	}
	if m.writeResult != nil {
		docsCreated = len(m.writeResult.FilesWritten)
		totalBytes = m.writeResult.TotalBytes
	}

	summary := fmt.Sprintf(`Documents created: %d
Total size:       %s
Time:             %s
Tokens used:      %d

Output: %s`,
		docsCreated,
		gemini.FormatSize(totalBytes),
		formatDuration(elapsed),
		tokensUsed,
		m.outputDir,
	)

	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSuccess).
		Padding(1, 2).
		Render(summary)

	// List created files
	var files strings.Builder
	if m.writeResult != nil && len(m.writeResult.FilesWritten) > 0 {
		files.WriteString(MutedStyle.Render("\nCreated files:\n"))
		for _, f := range m.writeResult.FilesWritten {
			files.WriteString(MutedStyle.Render("  - " + filepath.Base(f) + "\n"))
		}
	}

	hint := MutedStyle.Render("\n[a] Another transcription  [q] Quit")

	return BoxStyle.Render(title + "\n\n" + summaryBox + files.String() + hint)
}

// renderError renders the error screen
func (m TranscribeModel) renderError() string {
	title := ErrorStyle.Render("Error")

	errorBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorError).
		Padding(1, 2).
		Render(m.errorMessage)

	hint := MutedStyle.Render("\n[r] Retry  [q] Quit")

	return BoxStyle.Render(title + "\n\n" + errorBox + hint)
}

// renderHelp renders context-sensitive help
func (m TranscribeModel) renderHelp() string {
	var keys []string

	switch m.step {
	case TStepSelectSource, TStepSelectModel, TStepSelectOrganization:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "enter", "Select")
	case TStepSelectOptions:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "space", "Toggle")
		keys = append(keys, "enter", "Continue")
	case TStepSelectFolder:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "enter", "Select")
		keys = append(keys, "h/l", "Go up/down")
	case TStepEnterPattern, TStepConfigureOutput:
		keys = append(keys, "enter", "Confirm")
	case TStepConfirm:
		keys = append(keys, "y", "Yes")
		keys = append(keys, "n", "No")
		keys = append(keys, "tab", "Switch")
	}

	if m.step != TStepTranscribing && m.step != TStepWriting && m.step != TStepComplete && m.step != TStepError {
		keys = append(keys, "esc", "Back")
		keys = append(keys, "q", "Quit")
	}

	if len(keys) == 0 {
		return ""
	}

	helpStyle := lipgloss.NewStyle().Foreground(ColorMuted)
	keyStyle := lipgloss.NewStyle().Foreground(ColorSubtle).Bold(true)

	var parts []string
	for i := 0; i < len(keys); i += 2 {
		parts = append(parts, keyStyle.Render(keys[i])+" "+helpStyle.Render(keys[i+1]))
	}

	return helpStyle.Render(strings.Join(parts, "  |  "))
}

// Getter methods for external access
func (m TranscribeModel) IsQuitting() bool { return m.quitting }
func (m TranscribeModel) BackToMenu() bool { return m.backToMenu }
func (m TranscribeModel) HasError() bool   { return m.step == TStepError }
func (m TranscribeModel) GetError() string { return m.errorMessage }
func (m TranscribeModel) IsComplete() bool { return m.step == TStepComplete }

func formatDuration(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
}

// RunTranscribeUI runs the transcription UI and returns the result
func RunTranscribeUI() (continueApp bool, err error) {
	model := NewTranscribeModel()
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	m := finalModel.(TranscribeModel)
	if m.BackToMenu() {
		return true, nil
	}
	if m.HasError() {
		return false, fmt.Errorf("%s", m.GetError())
	}
	return false, nil
}
