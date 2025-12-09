package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"capycut/gemini"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/table"
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
	TStepSelectProvider
	TStepSelectModel
	TStepSelectOrganization
	TStepSelectOptions
	TStepConfirm
	TStepTranscribing
	TStepWriting
	TStepComplete
	TStepError
)

// FileEntry represents a file or directory in the file browser
type FileEntry struct {
	Name    string
	Path    string
	IsDir   bool
	Size    int64
	ModTime time.Time
}

// providerOption represents an AI provider option
type providerOption struct {
	name     string
	provider gemini.Provider
	desc     string
}

// modelOption represents a model option for a provider
type modelOption struct {
	name  string
	value string
	desc  string
}

// TranscribeModel is the Bubble Tea model for the transcription workflow
type TranscribeModel struct {
	// Current step
	step TranscribeStep

	// UI Components
	fileTable table.Model
	textInput textinput.Model
	spinner   spinner.Model
	progress  progress.Model

	// File browser state
	currentDir string
	files      []FileEntry
	tableReady bool

	// Selection state
	selectedSource        string // "folder" or "pattern"
	selectedFolder        string
	inputPattern          string
	outputDir             string
	selectedProvider      gemini.Provider
	selectedProviderIndex int
	availableProviders    []providerOption
	selectedModel         string
	selectedModelIndex    int
	availableModels       []modelOption
	orgMode               string // "chapters", "combine", "pages"
	orgModeIndex          int
	options               []bool // formatting, images, frontmatter, toc, index, overwrite
	optionIndex           int

	// Image data
	images     []string
	totalSize  int64
	imageCount int

	// Progress tracking
	transcribeProgress float64
	currentBatch       int
	totalBatches       int
	statusMessage      string

	// AI Agent status tracking
	aiStatus      gemini.ProgressStatus
	aiProvider    string
	aiModel       string
	aiMessage     string
	aiDetail      string
	aiTokensUsed  int
	aiStage       int
	aiTotalStages int

	// Unified AI Feed for transparency
	aiFeed *AIFeed

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

// aiProgressMsg carries AI progress updates from the transcription goroutine
type aiProgressMsg struct {
	status       gemini.ProgressStatus
	provider     string
	model        string
	message      string
	detail       string
	progress     float64
	currentBatch int
	totalBatches int
	tokensUsed   int
	elapsed      string
	stage        int
	totalStages  int
	// Transparency info
	requestInfo  *gemini.AIRequestInfo
	responseInfo *gemini.AIResponseInfo
}

// convertStatusToFeedType converts gemini.ProgressStatus to AIFeedMessageType
func convertStatusToFeedType(status gemini.ProgressStatus) AIFeedMessageType {
	switch status {
	case gemini.StatusSendingRequest:
		return MsgTypeRequest
	case gemini.StatusParsingResponse, gemini.StatusComplete:
		return MsgTypeResponse
	case gemini.StatusWaitingResponse:
		return MsgTypeThinking
	case gemini.StatusError:
		return MsgTypeError
	default:
		return MsgTypeStatus
	}
}

// fileSelectedMsg is sent when a file/folder is selected
type fileSelectedMsg string

// Organization and additional options
var (
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

// getAvailableProviders returns a list of configured AI providers
func getAvailableProviders() []providerOption {
	var providers []providerOption

	// Check for Local LLM endpoint
	if os.Getenv("LLM_ENDPOINT") != "" || os.Getenv("IMAGE_LLM_ENDPOINT") != "" {
		endpoint := os.Getenv("LLM_ENDPOINT")
		if endpoint == "" {
			endpoint = os.Getenv("IMAGE_LLM_ENDPOINT")
		}
		providers = append(providers, providerOption{
			name:     fmt.Sprintf("Local LLM (%s)", endpoint),
			provider: gemini.ProviderLocal,
			desc:     "Free, runs on your machine",
		})
	}

	// Check for Azure Anthropic
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" {
		providers = append(providers, providerOption{
			name:     "Azure Anthropic (Claude)",
			provider: gemini.ProviderAzureAnthropic,
			desc:     "Claude models via Azure",
		})
	}

	// Check for Gemini API key
	if os.Getenv("GEMINI_API_KEY") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
		providers = append(providers, providerOption{
			name:     "Google Gemini",
			provider: gemini.ProviderGemini,
			desc:     "Cloud API",
		})
	}

	// If no providers configured, show all as unconfigured
	if len(providers) == 0 {
		providers = []providerOption{
			{"Local LLM (not configured)", gemini.ProviderLocal, "Set LLM_ENDPOINT"},
			{"Azure Anthropic (not configured)", gemini.ProviderAzureAnthropic, "Set AZURE_ANTHROPIC_ENDPOINT"},
			{"Google Gemini (not configured)", gemini.ProviderGemini, "Set GEMINI_API_KEY"},
		}
	}

	return providers
}

// getModelsForProvider returns available models for a given provider
func getModelsForProvider(provider gemini.Provider) []modelOption {
	switch provider {
	case gemini.ProviderLocal:
		defaultModel := os.Getenv("LLM_MODEL")
		if defaultModel == "" {
			defaultModel = os.Getenv("IMAGE_LLM_MODEL")
		}
		if defaultModel == "" {
			defaultModel = "local-model"
		}
		models := []modelOption{
			{fmt.Sprintf("Default (%s)", defaultModel), defaultModel, "From environment"},
		}
		// Add common vision models
		commonModels := []modelOption{
			{"LLaVA", "llava", "Vision model"},
			{"LLaVA 13B", "llava:13b", "Larger LLaVA"},
			{"BakLLaVA", "bakllava", "Vision model"},
			{"Qwen-VL", "qwen-vl", "Qwen Vision"},
			{"MiniCPM-V", "minicpm-v", "Compact vision"},
		}
		// Don't duplicate if default is one of the common models
		for _, m := range commonModels {
			if m.value != defaultModel {
				models = append(models, m)
			}
		}
		return models

	case gemini.ProviderAzureAnthropic:
		envModel := os.Getenv("AZURE_ANTHROPIC_MODEL")
		var models []modelOption
		if envModel != "" {
			models = append(models, modelOption{
				fmt.Sprintf("From env (%s)", envModel), envModel, "Configured model",
			})
		}
		models = append(models,
			modelOption{"Claude Sonnet 4", "claude-sonnet-4-20250514", "Latest Sonnet"},
			modelOption{"Claude 3.5 Sonnet", "claude-3-5-sonnet-20241022", "Previous Sonnet"},
			modelOption{"Claude 3 Opus", "claude-3-opus-20240229", "Most capable"},
			modelOption{"Claude 3 Haiku", "claude-3-haiku-20240307", "Fast & light"},
		)
		return models

	case gemini.ProviderGemini:
		return []modelOption{
			{"Gemini 3 Pro", gemini.ModelGemini3Pro, "Latest, most capable"},
			{"Gemini 2.5 Flash", gemini.ModelGemini25Flash, "Fast & efficient"},
		}

	default:
		return []modelOption{
			{"Default", "", "Auto-select"},
		}
	}
}

// NewTranscribeModel creates a new transcription model
func NewTranscribeModel() TranscribeModel {
	// Initialize file table
	columns := []table.Column{
		{Title: "Type", Width: 6},
		{Title: "Name", Width: 40},
		{Title: "Size", Width: 12},
		{Title: "Modified", Width: 20},
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
		BorderForeground(ColorBorder).
		BorderBottom(true).
		Bold(true).
		Foreground(ColorPrimary)
	s.Selected = s.Selected.
		Foreground(lipgloss.Color("#FFFFFF")).
		Background(ColorPrimary).
		Bold(true)
	s.Cell = s.Cell.Foreground(ColorText)
	t.SetStyles(s)

	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "./images/*.png"
	ti.CharLimit = 256
	ti.Width = 50

	// Initialize spinner with custom style
	sp := spinner.New()
	sp.Spinner = spinner.Spinner{
		Frames: []string{"( o.o ) ", "( o.o )>", "( o.o)>>", "(o.o )>>", "( o.o)> ", "( o.o ) "},
		FPS:    time.Second / 8,
	}
	sp.Style = lipgloss.NewStyle().Foreground(ColorCapybara)

	// Initialize progress bar
	p := progress.New(
		progress.WithDefaultGradient(),
		progress.WithWidth(50),
	)

	// Get starting directory
	startDir, err := os.Getwd()
	if err != nil {
		startDir = "/"
	}

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize unified AI feed for transparency
	aiFeed := NewAIFeed(70, 10)

	return TranscribeModel{
		step:       TStepSelectSource,
		fileTable:  t,
		textInput:  ti,
		spinner:    sp,
		progress:   p,
		currentDir: startDir,
		options:    make([]bool, len(additionalOptions)),
		width:      80,
		height:     24,
		ctx:        ctx,
		cancel:     cancel,
		aiFeed:     aiFeed,
	}
}

// loadDirectoryEntries reads directory contents and returns file entries
func loadDirectoryEntries(dir string) ([]FileEntry, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	files := []FileEntry{}

	// Add parent directory entry if not at root
	if dir != "/" {
		files = append(files, FileEntry{
			Name:  "..",
			Path:  filepath.Dir(dir),
			IsDir: true,
		})
	}

	// Read all entries
	for _, entry := range entries {
		// Skip hidden files
		if strings.HasPrefix(entry.Name(), ".") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileEntry{
			Name:    entry.Name(),
			Path:    filepath.Join(dir, entry.Name()),
			IsDir:   entry.IsDir(),
			Size:    info.Size(),
			ModTime: info.ModTime(),
		})
	}

	// Sort: directories first, then by name
	sort.Slice(files, func(i, j int) bool {
		// Keep ".." at the top
		if files[i].Name == ".." {
			return true
		}
		if files[j].Name == ".." {
			return false
		}
		// Directories before files
		if files[i].IsDir != files[j].IsDir {
			return files[i].IsDir
		}
		// Alphabetical
		return strings.ToLower(files[i].Name) < strings.ToLower(files[j].Name)
	})

	return files, nil
}

// buildTableRows creates table rows from file entries
func buildTableRows(files []FileEntry) []table.Row {
	rows := make([]table.Row, len(files))
	for i, f := range files {
		typeIcon := "FILE"
		if f.IsDir {
			typeIcon = "DIR"
		}
		if f.Name == ".." {
			typeIcon = "UP"
		}

		sizeStr := ""
		if !f.IsDir {
			sizeStr = formatFileSize(f.Size)
		}

		modStr := ""
		if f.Name != ".." && !f.ModTime.IsZero() {
			modStr = f.ModTime.Format("Jan 02 15:04")
		}

		rows[i] = table.Row{typeIcon, f.Name, sizeStr, modStr}
	}
	return rows
}

// formatFileSize formats bytes into human readable format
func formatFileSize(bytes int64) string {
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

// Init initializes the model
func (m TranscribeModel) Init() tea.Cmd {
	return m.spinner.Tick
}

// Update handles messages
func (m TranscribeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = m.width - 20

		// Dynamically adjust table height based on available terminal space
		// Reserve space for header (~10 lines), footer (~4 lines), and box padding (~4 lines)
		availableHeight := m.height - 18
		if availableHeight < 8 {
			availableHeight = 8
		}
		if availableHeight > 25 {
			availableHeight = 25
		}
		m.fileTable.SetHeight(availableHeight)

		// Adjust table column widths based on terminal width
		nameWidth := m.width - 60 // Leave space for other columns and borders
		if nameWidth < 20 {
			nameWidth = 20
		}
		if nameWidth > 80 {
			nameWidth = 80
		}
		m.fileTable.SetColumns([]table.Column{
			{Title: "Type", Width: 6},
			{Title: "Name", Width: nameWidth},
			{Title: "Size", Width: 12},
			{Title: "Modified", Width: 18},
		})

		// Resize AI feed for transcribing view
		chatHeight := m.height - 22 // Reserve space for header, progress bar, stats
		if chatHeight < 6 {
			chatHeight = 6
		}
		if chatHeight > 15 {
			chatHeight = 15
		}
		m.aiFeed.SetSize(m.width-12, chatHeight)

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

	case aiProgressMsg:
		m.aiStatus = msg.status
		m.aiProvider = msg.provider
		m.aiModel = msg.model
		m.aiMessage = msg.message
		m.aiDetail = msg.detail
		m.transcribeProgress = msg.progress
		m.currentBatch = msg.currentBatch
		m.totalBatches = msg.totalBatches
		m.aiTokensUsed = msg.tokensUsed
		m.aiStage = msg.stage
		m.aiTotalStages = msg.totalStages

		// Add message to unified AI feed with transparency info
		if msg.message != "" {
			feedMsg := AIFeedMessage{
				Timestamp: time.Now(),
				Type:      convertStatusToFeedType(msg.status),
				Provider:  msg.provider,
				Model:     msg.model,
				Title:     msg.message,
				Details:   []string{msg.detail},
			}

			// Add request info if present (compact - just image count and size)
			if msg.requestInfo != nil {
				feedMsg.RequestInfo = &AIRequestInfo{
					ImageCount:    msg.requestInfo.ImageCount,
					TotalDataSize: msg.requestInfo.TotalDataSize,
				}
			}

			// Add response info if present (compact - no raw content)
			if msg.responseInfo != nil {
				feedMsg.ResponseInfo = &AIResponseInfo{
					StatusCode:     msg.responseInfo.StatusCode,
					StatusText:     msg.responseInfo.StatusText,
					Latency:        msg.responseInfo.Latency,
					TokensTotal:    msg.responseInfo.TokensTotal,
					ItemsProcessed: msg.responseInfo.ItemsProcessed,
					ErrorMessage:   msg.responseInfo.ErrorMessage,
				}
			}

			m.aiFeed.AddMessage(feedMsg)
		}

		// Continue listening for more progress messages
		var cmd tea.Cmd = m.progress.SetPercent(msg.progress)
		if transcribeProgressChan != nil && msg.status != gemini.StatusComplete && msg.status != gemini.StatusError {
			cmd = tea.Batch(cmd, waitForProgress(transcribeProgressChan))
		}
		return m, cmd

	case fileSelectedMsg:
		m.selectedFolder = string(msg)
		m.step = TStepLoadingImages
		return m, m.loadImages([]string{string(msg)})
	}

	// Update sub-components based on step
	switch m.step {
	case TStepSelectFolder:
		var cmd tea.Cmd
		m.fileTable, cmd = m.fileTable.Update(msg)
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
				// Load current directory into the table
				files, err := loadDirectoryEntries(m.currentDir)
				if err == nil {
					m.files = files
					m.fileTable.SetRows(buildTableRows(m.files))
				}
				m.tableReady = true
				return m, nil
			} else {
				m.selectedSource = "pattern"
				m.step = TStepEnterPattern
				m.textInput.Placeholder = "./images/*.png or /path/to/folder"
				m.textInput.SetValue("")
				m.textInput.Focus()
				return m, textinput.Blink
			}
		}

	case TStepSelectFolder:
		switch msg.String() {
		case "enter":
			// Get selected row
			selectedRow := m.fileTable.Cursor()
			if selectedRow >= 0 && selectedRow < len(m.files) {
				selected := m.files[selectedRow]
				if selected.IsDir {
					// Navigate into directory or go back
					files, err := loadDirectoryEntries(selected.Path)
					if err == nil {
						m.files = files
						m.currentDir = selected.Path
						m.fileTable.SetRows(buildTableRows(m.files))
					}
				} else {
					// File selected - use its parent directory
					m.selectedFolder = m.currentDir
					return m, func() tea.Msg { return fileSelectedMsg(m.currentDir) }
				}
			}
		case "s", " ":
			// Select current directory
			m.selectedFolder = m.currentDir
			return m, func() tea.Msg { return fileSelectedMsg(m.currentDir) }
		default:
			// Forward all other keys (up/down/j/k/etc.) to the table for navigation
			var cmd tea.Cmd
			m.fileTable, cmd = m.fileTable.Update(msg)
			return m, cmd
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
		default:
			// Forward all other keys to the text input for typing
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

	case TStepConfigureOutput:
		switch msg.String() {
		case "enter":
			m.outputDir = m.textInput.Value()
			if m.outputDir == "" {
				m.outputDir = "./output"
			}
			// Initialize available providers and go to provider selection
			m.availableProviders = getAvailableProviders()
			m.selectedProviderIndex = 0
			m.step = TStepSelectProvider
		default:
			// Forward all other keys to the text input for typing
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

	case TStepSelectProvider:
		switch msg.String() {
		case "up", "k":
			if m.selectedProviderIndex > 0 {
				m.selectedProviderIndex--
			}
		case "down", "j":
			if m.selectedProviderIndex < len(m.availableProviders)-1 {
				m.selectedProviderIndex++
			}
		case "enter":
			m.selectedProvider = m.availableProviders[m.selectedProviderIndex].provider
			// Load models for selected provider
			m.availableModels = getModelsForProvider(m.selectedProvider)
			m.selectedModelIndex = 0
			m.step = TStepSelectModel
		}

	case TStepSelectModel:
		switch msg.String() {
		case "up", "k":
			if m.selectedModelIndex > 0 {
				m.selectedModelIndex--
			}
		case "down", "j":
			if m.selectedModelIndex < len(m.availableModels)-1 {
				m.selectedModelIndex++
			}
		case "enter":
			m.selectedModel = m.availableModels[m.selectedModelIndex].value
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
	case TStepSelectProvider:
		m.step = TStepConfigureOutput
		m.textInput.SetValue(m.outputDir)
		m.textInput.Focus()
	case TStepSelectModel:
		m.step = TStepSelectProvider
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

// waitForProgress waits for the next progress message from the channel
func waitForProgress(ch chan aiProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil // Channel closed
		}
		return msg
	}
}

// transcribeProgressChan holds the current progress channel
var transcribeProgressChan chan aiProgressMsg

// transcribeResultChan holds the result channel
var transcribeResultChan chan transcribeResultMsg

// startTranscription begins the transcription process
func (m TranscribeModel) startTranscription() tea.Cmd {
	// Create channels for progress and result
	progressChan := make(chan aiProgressMsg, 100)
	resultChan := make(chan transcribeResultMsg, 1)

	// Store channels for access in Update
	transcribeProgressChan = progressChan
	transcribeResultChan = resultChan

	// Capture values needed in the goroutine
	provider := m.selectedProvider
	model := m.selectedModel
	images := m.images
	outputDir := m.outputDir
	orgMode := m.orgMode
	options := m.options

	// Start the transcription goroutine
	go func() {
		defer close(progressChan)

		// Create client based on selected provider
		var client *gemini.Client
		var err error

		debug := os.Getenv("CAPYCUT_DEBUG") != ""

		switch provider {
		case gemini.ProviderLocal:
			endpoint := os.Getenv("LLM_ENDPOINT")
			if endpoint == "" {
				endpoint = os.Getenv("IMAGE_LLM_ENDPOINT")
			}
			if endpoint == "" {
				resultChan <- transcribeResultMsg{err: fmt.Errorf("LLM_ENDPOINT not configured")}
				return
			}
			client, err = gemini.NewLocalClient(endpoint, model, gemini.WithDebug(debug))

		case gemini.ProviderAzureAnthropic:
			endpoint := os.Getenv("AZURE_ANTHROPIC_ENDPOINT")
			apiKey := os.Getenv("AZURE_ANTHROPIC_API_KEY")
			if endpoint == "" || apiKey == "" {
				resultChan <- transcribeResultMsg{err: fmt.Errorf("Azure Anthropic not configured")}
				return
			}
			client, err = gemini.NewAzureAnthropicClient(endpoint, apiKey, model, gemini.WithDebug(debug))

		case gemini.ProviderGemini:
			apiKey := os.Getenv("GEMINI_API_KEY")
			if apiKey == "" {
				apiKey = os.Getenv("GOOGLE_API_KEY")
			}
			if apiKey == "" {
				resultChan <- transcribeResultMsg{err: fmt.Errorf("Gemini API key not configured")}
				return
			}
			client, err = gemini.NewClient(apiKey, gemini.WithDebug(debug))

		default:
			// Fall back to auto-detection
			client, err = gemini.NewClientFromEnv()
		}

		if err != nil {
			resultChan <- transcribeResultMsg{err: err}
			return
		}

		req := &gemini.TranscribeRequest{
			Images:                   images,
			OutputDir:                outputDir,
			Model:                    model,
			DetectChapters:           orgMode == "chapters",
			CombinePages:             orgMode == "combine",
			PreserveFormatting:       options[0],
			IncludeImageDescriptions: options[1],
		}

		// Progress callback that sends updates through the channel
		onProgress := func(update gemini.ProgressUpdate) {
			elapsed := ""
			if update.Elapsed > 0 {
				elapsed = formatDuration(update.Elapsed)
			}
			select {
			case progressChan <- aiProgressMsg{
				status:       update.Status,
				provider:     update.Provider,
				model:        update.Model,
				message:      update.Message,
				detail:       update.Detail,
				progress:     update.Progress,
				currentBatch: update.CurrentBatch,
				totalBatches: update.TotalBatches,
				tokensUsed:   update.TokensUsed,
				elapsed:      elapsed,
				stage:        update.Stage,
				totalStages:  update.TotalStages,
				requestInfo:  update.RequestInfo,
				responseInfo: update.ResponseInfo,
			}:
			default:
				// Don't block if channel is full
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()

		resp, err := client.TranscribeImagesWithProgress(ctx, req, onProgress)
		resultChan <- transcribeResultMsg{response: resp, err: err}
	}()

	// Return commands to listen for both progress and result
	return tea.Batch(
		waitForProgress(progressChan),
		func() tea.Msg {
			return <-resultChan
		},
	)
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

	// Only show header if we have enough vertical space
	if m.height > 30 {
		b.WriteString(GetCapybaraHeader())
		b.WriteString("\n")
	} else {
		// Compact header for smaller terminals
		b.WriteString(lipgloss.NewStyle().
			Foreground(ColorCapybara).
			Bold(true).
			Render("CAPYCUT") + "\n\n")
	}

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
	case TStepSelectProvider:
		b.WriteString(m.renderProviderSelection())
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
		{"Provider", m.step >= TStepSelectProvider, m.step > TStepSelectProvider},
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

// renderFolderPicker renders the folder picker using a table
func (m TranscribeModel) renderFolderPicker() string {
	title := TitleStyle.Render("Select a folder containing images")
	desc := MutedStyle.Render("Press SPACE or 's' to select current folder, ENTER to open")

	// Breadcrumb showing current directory path
	currentDir := m.currentDir
	if currentDir == "" {
		currentDir = "/"
	}

	// Style the breadcrumb path - truncate if too long
	maxPathLen := m.width - 30
	if maxPathLen < 20 {
		maxPathLen = 20
	}
	displayPath := currentDir
	if len(displayPath) > maxPathLen {
		displayPath = "..." + displayPath[len(displayPath)-maxPathLen+3:]
	}

	pathStyle := lipgloss.NewStyle().Foreground(ColorSecondary).Bold(true)

	breadcrumbBox := lipgloss.NewStyle().
		Foreground(ColorMuted).
		Render("Location: ") + pathStyle.Render(displayPath)

	// File count
	fileCountHint := MutedStyle.Render(fmt.Sprintf("%d items", len(m.files)))

	// Table container style for full width
	tableStyle := lipgloss.NewStyle().
		Width(m.width - 8).
		MaxWidth(m.width - 8)

	// Use constrained box style to prevent overflow
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Width(m.width - 4).
		MaxHeight(m.height - 12) // Leave room for header and help

	return boxStyle.Render(
		title + "\n" +
			desc + "\n\n" +
			breadcrumbBox + "  " + fileCountHint + "\n\n" +
			tableStyle.Render(m.fileTable.View()),
	)
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

// renderProviderSelection renders the provider selection
func (m TranscribeModel) renderProviderSelection() string {
	title := TitleStyle.Render("Select AI Provider")

	var items strings.Builder
	for i, opt := range m.availableProviders {
		cursor := "  "
		style := BodyStyle
		if i == m.selectedProviderIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		items.WriteString(style.Render(cursor+opt.name) +
			MutedStyle.Render(" - "+opt.desc) + "\n")
	}

	return BoxStyle.Render(title + "\n\n" + items.String())
}

// renderModelSelection renders the model selection
func (m TranscribeModel) renderModelSelection() string {
	title := TitleStyle.Render("Select Model")

	var items strings.Builder
	for i, opt := range m.availableModels {
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

	// Get provider and model names
	providerName := "Unknown"
	if m.selectedProviderIndex < len(m.availableProviders) {
		providerName = m.availableProviders[m.selectedProviderIndex].name
	}
	modelName := m.selectedModel
	if m.selectedModelIndex < len(m.availableModels) {
		modelName = m.availableModels[m.selectedModelIndex].name
	}

	// Summary
	summary := fmt.Sprintf(`Images:     %d files (%.2f MB)
Provider:   %s
Model:      %s
Output:     %s
Mode:       %s`,
		m.imageCount,
		float64(m.totalSize)/(1024*1024),
		providerName,
		modelName,
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

// renderTranscribing renders the transcription progress with detailed AI status
func (m TranscribeModel) renderTranscribing() string {
	title := TitleStyle.Render("Transcribing...")

	var content strings.Builder

	// Simple status line with spinner
	content.WriteString(m.spinner.View() + " ")
	if m.aiMessage != "" {
		content.WriteString(BodyStyle.Render(m.aiMessage))
	} else {
		content.WriteString(BodyStyle.Render("Processing..."))
	}
	content.WriteString("\n\n")

	// Progress bar
	content.WriteString(m.progress.View())
	content.WriteString("\n\n")

	// AI Activity Log (simple)
	feedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(m.width - 12)
	content.WriteString(feedStyle.Render(m.aiFeed.Render()))
	content.WriteString("\n\n")

	// Stats line
	var stats []string
	if m.totalBatches > 0 {
		stats = append(stats, fmt.Sprintf("Batch %d/%d", m.currentBatch, m.totalBatches))
	}
	elapsed := time.Since(m.startTime)
	stats = append(stats, fmt.Sprintf("Time: %s", formatDuration(elapsed)))
	if m.aiTokensUsed > 0 {
		stats = append(stats, fmt.Sprintf("Tokens: %d", m.aiTokensUsed))
	}
	statsStr := MutedStyle.Render(strings.Join(stats, " | "))
	content.WriteString(statsStr)

	return BoxStyle.Width(m.width - 4).Render(title + "\n\n" + content.String())
}

// renderAIAgentHeader renders a compact header showing the current AI agent
func (m TranscribeModel) renderAIAgentHeader() string {
	// Determine status icon and color
	var statusIcon string
	var statusStyle lipgloss.Style

	switch m.aiStatus {
	case gemini.StatusComplete:
		statusIcon = "[x]"
		statusStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	case gemini.StatusError:
		statusIcon = "[!]"
		statusStyle = lipgloss.NewStyle().Foreground(ColorError)
	case gemini.StatusProcessingBatch, gemini.StatusWaitingResponse, gemini.StatusSendingRequest:
		statusIcon = "[>]"
		statusStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
	case gemini.StatusParsingResponse:
		statusIcon = "[*]"
		statusStyle = lipgloss.NewStyle().Foreground(ColorSecondary)
	default:
		statusIcon = "[~]"
		statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	subtitleStyle := lipgloss.NewStyle().Foreground(ColorSubtle)

	// Header with spinner, provider name and model
	providerName := m.getProviderDisplayName()
	modelName := m.aiModel
	if modelName == "" {
		modelName = m.selectedModel
	}

	header := m.spinner.View() + " " + statusStyle.Render(statusIcon) + " " +
		titleStyle.Render(providerName)
	if modelName != "" {
		header += " " + subtitleStyle.Render("("+modelName+")")
	}

	// Stage indicator
	if m.aiTotalStages > 1 {
		stageInfo := MutedStyle.Render(fmt.Sprintf(" - Stage %d/%d", m.aiStage, m.aiTotalStages))
		header += stageInfo
	}

	return header
}

// getProviderDisplayName returns a user-friendly provider name
func (m TranscribeModel) getProviderDisplayName() string {
	switch m.aiProvider {
	case "gemini":
		return "Google Gemini"
	case "local":
		return "Local LLM"
	case "azure_anthropic":
		return "Azure Anthropic"
	default:
		if m.aiProvider != "" {
			return m.aiProvider
		}
		// Fallback based on environment
		return "AI Engine"
	}
}

// renderWriting renders the writing progress
func (m TranscribeModel) renderWriting() string {
	title := TitleStyle.Render("Writing Files...")

	var content strings.Builder

	// Show AI processing summary with transparency info
	summaryStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSuccess).
		Padding(0, 2).
		Width(m.width - 10)

	var summaryLines []string
	summaryLines = append(summaryLines, SuccessStyle.Render("[x] AI Processing Complete"))
	if m.aiProvider != "" {
		summaryLines = append(summaryLines, MutedStyle.Render("    Provider: "+m.getProviderDisplayName()))
	}
	if m.aiModel != "" {
		summaryLines = append(summaryLines, MutedStyle.Render("    Model: "+m.aiModel))
	}
	if m.aiTokensUsed > 0 {
		summaryLines = append(summaryLines, MutedStyle.Render(fmt.Sprintf("    Tokens used: %d", m.aiTokensUsed)))
	}
	if m.result != nil {
		summaryLines = append(summaryLines, MutedStyle.Render(fmt.Sprintf("    Pages processed: %d", m.result.TotalPages)))
		summaryLines = append(summaryLines, MutedStyle.Render(fmt.Sprintf("    Documents created: %d", len(m.result.Documents))))
	}
	if m.totalBatches > 0 {
		summaryLines = append(summaryLines, MutedStyle.Render(fmt.Sprintf("    Batches: %d", m.totalBatches)))
	}

	content.WriteString(summaryStyle.Render(strings.Join(summaryLines, "\n")))
	content.WriteString("\n\n")

	// Show final AI transparency log summary
	content.WriteString(SubtitleStyle.Render("AI Request History"))
	content.WriteString("\n")
	feedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(m.width - 12).
		MaxHeight(6)
	content.WriteString(feedStyle.Render(m.aiFeed.Render()))
	content.WriteString("\n\n")

	// Writing status
	spinnerView := m.spinner.View()
	status := BodyStyle.Render("Saving markdown files to disk...")
	content.WriteString(spinnerView + " " + status)

	return BoxStyle.Render(title + "\n\n" + content.String())
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

	// AI Summary section
	var aiSummary strings.Builder
	aiSummary.WriteString(SubtitleStyle.Render("AI Processing Summary") + "\n")
	aiSummary.WriteString(MutedStyle.Render(fmt.Sprintf("  Provider:    %s\n", m.getProviderDisplayName())))
	if m.aiModel != "" {
		aiSummary.WriteString(MutedStyle.Render(fmt.Sprintf("  Model:       %s\n", m.aiModel)))
	}
	aiSummary.WriteString(MutedStyle.Render(fmt.Sprintf("  Tokens:      %d\n", tokensUsed)))
	aiSummary.WriteString(MutedStyle.Render(fmt.Sprintf("  Batches:     %d\n", m.totalBatches)))
	aiSummary.WriteString(MutedStyle.Render(fmt.Sprintf("  Images:      %d\n", m.imageCount)))

	// Results section
	summary := fmt.Sprintf(`Documents created: %d
Total size:       %s
Processing time:  %s

Output: %s`,
		docsCreated,
		gemini.FormatSize(totalBytes),
		formatDuration(elapsed),
		m.outputDir,
	)

	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSuccess).
		Padding(1, 2).
		Render(summary)

	// List created files (limit to first 5)
	var files strings.Builder
	if m.writeResult != nil && len(m.writeResult.FilesWritten) > 0 {
		files.WriteString(MutedStyle.Render("\nCreated files:\n"))
		maxShow := 5
		for i, f := range m.writeResult.FilesWritten {
			if i >= maxShow {
				files.WriteString(MutedStyle.Render(fmt.Sprintf("  ... and %d more\n", len(m.writeResult.FilesWritten)-maxShow)))
				break
			}
			files.WriteString(MutedStyle.Render("  - " + filepath.Base(f) + "\n"))
		}
	}

	hint := MutedStyle.Render("\n[a] Another transcription  [q] Quit")

	return BoxStyle.Render(title + "\n\n" + aiSummary.String() + "\n" + summaryBox + files.String() + hint)
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
	case TStepSelectSource, TStepSelectProvider, TStepSelectModel, TStepSelectOrganization:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "enter", "Select")
	case TStepSelectOptions:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "space", "Toggle")
		keys = append(keys, "enter", "Continue")
	case TStepSelectFolder:
		keys = append(keys, "j/k/arrows", "Navigate")
		keys = append(keys, "enter", "Open folder")
		keys = append(keys, "space/s", "Select this folder")
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
