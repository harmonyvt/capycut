// Package tui provides a beautiful terminal UI for CapyCut using Charm libraries
package tui

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"capycut/ai"
	"capycut/video"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// ClipStep represents the current step in the clip workflow
type ClipStep int

const (
	CStepSelectVideo ClipStep = iota
	CStepLoadingVideo
	CStepSelectProvider
	CStepEnterDescription
	CStepParsing
	CStepConfirm
	CStepClipping
	CStepComplete
	CStepError
)

// ClipModel is the Bubble Tea model for the video clipping workflow
type ClipModel struct {
	// Current step
	step ClipStep

	// UI Components
	textInput textinput.Model
	spinner   spinner.Model
	progress  progress.Model

	// Video selection
	videoPath string
	videoInfo *video.VideoInfo

	// Provider selection
	selectedProvider      ai.Provider
	selectedProviderIndex int
	availableProviders    []clipProviderOption

	// Clip description
	clipDescription string

	// Parsing result
	clipRequest *ai.ClipRequest
	outputPath  string

	// AI Agent status tracking
	aiProvider string
	aiModel    string
	aiMessage  string
	aiDetail   string

	// Unified AI Feed for transparency
	aiFeed *AIFeed

	// Results
	errorMessage string
	startTime    time.Time
	outputSize   int64

	// Dimensions
	width  int
	height int

	// State flags
	quitting   bool
	backToMenu bool

	// Menu indices
	providerIndex int
	confirmIndex  int

	// Parser reference
	parser *ai.Parser

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc
}

// clipProviderOption represents an AI provider option for clipping
type clipProviderOption struct {
	name     string
	provider ai.Provider
	desc     string
}

// Messages
type clipVideoInfoMsg struct {
	info *video.VideoInfo
	err  error
}

type clipParseResultMsg struct {
	result *ai.ClipRequest
	err    error
}

type clipCompleteMsg struct {
	outputPath string
	outputSize int64
	err        error
}

type clipProgressMsg struct {
	status       ai.ParserProgressStatus
	provider     string
	model        string
	message      string
	detail       string
	requestInfo  *ai.ParserRequestInfo
	responseInfo *ai.ParserResponseInfo
}

// NewClipModel creates a new clip model
func NewClipModel(videoPath string) ClipModel {
	// Initialize text input
	ti := textinput.New()
	ti.Placeholder = "e.g., 'first 2 minutes' or 'from 1:30 to 3:45'"
	ti.CharLimit = 500
	ti.Width = 60

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

	ctx, cancel := context.WithCancel(context.Background())

	// Initialize unified AI feed for transparency
	aiFeed := NewAIFeed(70, 8)

	// Get available providers
	providers := getClipProviders()

	return ClipModel{
		step:               CStepLoadingVideo,
		textInput:          ti,
		spinner:            sp,
		progress:           p,
		videoPath:          videoPath,
		availableProviders: providers,
		width:              80,
		height:             24,
		ctx:                ctx,
		cancel:             cancel,
		aiFeed:             aiFeed,
	}
}

// getClipProviders returns available AI providers for clipping
func getClipProviders() []clipProviderOption {
	var providers []clipProviderOption

	// Check for Local LLM endpoint
	if os.Getenv("LLM_ENDPOINT") != "" {
		endpoint := os.Getenv("LLM_ENDPOINT")
		providers = append(providers, clipProviderOption{
			name:     fmt.Sprintf("Local LLM (%s)", endpoint),
			provider: ai.ProviderLocal,
			desc:     "Free, runs on your machine",
		})
	}

	// Check for Azure Anthropic
	if os.Getenv("AZURE_ANTHROPIC_ENDPOINT") != "" && os.Getenv("AZURE_ANTHROPIC_API_KEY") != "" {
		providers = append(providers, clipProviderOption{
			name:     "Azure Anthropic (Claude)",
			provider: ai.ProviderAzureAnthropic,
			desc:     "Claude models via Azure",
		})
	}

	// Check for Azure OpenAI
	if os.Getenv("AZURE_OPENAI_ENDPOINT") != "" && os.Getenv("AZURE_OPENAI_API_KEY") != "" {
		providers = append(providers, clipProviderOption{
			name:     "Azure OpenAI",
			provider: ai.ProviderAzure,
			desc:     "GPT models via Azure",
		})
	}

	return providers
}

// Init initializes the model
func (m ClipModel) Init() tea.Cmd {
	return tea.Batch(
		m.spinner.Tick,
		m.loadVideoInfo(),
	)
}

// loadVideoInfo loads video information
func (m ClipModel) loadVideoInfo() tea.Cmd {
	return func() tea.Msg {
		info, err := video.GetVideoInfo(m.videoPath)
		return clipVideoInfoMsg{info: info, err: err}
	}
}

// Update handles messages
func (m ClipModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.progress.Width = m.width - 20
		m.aiFeed.SetSize(m.width-12, 8)
		return m, nil

	case tea.KeyMsg:
		// Global key handlers
		switch msg.String() {
		case "ctrl+c", "q":
			if m.step != CStepParsing && m.step != CStepClipping {
				m.quitting = true
				m.cancel()
				return m, tea.Quit
			}
		case "esc":
			if m.step == CStepParsing || m.step == CStepClipping {
				m.cancel()
				m.errorMessage = "Cancelled by user"
				m.step = CStepError
				return m, nil
			}
			return m.goBack()
		}

		return m.handleStepInput(msg)

	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case clipVideoInfoMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = CStepError
			return m, nil
		}
		m.videoInfo = msg.info
		if len(m.availableProviders) == 1 {
			m.selectedProvider = m.availableProviders[0].provider
			m.step = CStepEnterDescription
			m.textInput.Focus()
			return m, textinput.Blink
		}
		m.step = CStepSelectProvider
		return m, nil

	case clipProgressMsg:
		m.aiProvider = msg.provider
		m.aiModel = msg.model
		m.aiMessage = msg.message
		m.aiDetail = msg.detail

		// Add to unified AI feed with transparency info
		feedMsg := AIFeedMessage{
			Timestamp: time.Now(),
			Type:      convertParserStatusToFeedType(msg.status),
			Provider:  msg.provider,
			Model:     msg.model,
			Title:     msg.message,
			Details:   []string{msg.detail},
		}

		// Add request info if present (compact)
		if msg.requestInfo != nil {
			feedMsg.RequestInfo = &AIRequestInfo{
				DataSummary: msg.requestInfo.UserInput,
			}
		}

		// Add response info if present (compact - no raw JSON)
		if msg.responseInfo != nil {
			feedMsg.ResponseInfo = &AIResponseInfo{
				StatusCode:   msg.responseInfo.StatusCode,
				StatusText:   msg.responseInfo.StatusText,
				Latency:      msg.responseInfo.Latency,
				ErrorMessage: msg.responseInfo.ErrorMessage,
			}
		}

		m.aiFeed.AddMessage(feedMsg)

		// Continue listening for more progress if not complete/error
		if clipProgressChan != nil && msg.status != ai.ParserStatusComplete && msg.status != ai.ParserStatusError {
			return m, waitForClipProgress(clipProgressChan)
		}
		return m, nil

	case clipParseResultMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = CStepError
			return m, nil
		}
		m.clipRequest = msg.result
		m.outputPath = video.GenerateOutputPath(m.videoPath, msg.result.StartTime, msg.result.EndTime)
		m.step = CStepConfirm
		return m, nil

	case clipCompleteMsg:
		if msg.err != nil {
			m.errorMessage = msg.err.Error()
			m.step = CStepError
			return m, nil
		}
		m.outputPath = msg.outputPath
		m.outputSize = msg.outputSize
		m.step = CStepComplete
		return m, nil
	}

	// Update sub-components based on step
	switch m.step {
	case CStepEnterDescription:
		var cmd tea.Cmd
		m.textInput, cmd = m.textInput.Update(msg)
		return m, cmd
	}

	return m, nil
}

// handleStepInput handles keyboard input for specific steps
func (m ClipModel) handleStepInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.step {
	case CStepSelectProvider:
		switch msg.String() {
		case "up", "k":
			if m.providerIndex > 0 {
				m.providerIndex--
			}
		case "down", "j":
			if m.providerIndex < len(m.availableProviders)-1 {
				m.providerIndex++
			}
		case "enter":
			m.selectedProvider = m.availableProviders[m.providerIndex].provider
			m.step = CStepEnterDescription
			m.textInput.Focus()
			return m, textinput.Blink
		}

	case CStepEnterDescription:
		switch msg.String() {
		case "enter":
			desc := m.textInput.Value()
			if desc != "" {
				m.clipDescription = desc
				m.step = CStepParsing
				m.startTime = time.Now()
				return m, m.startParsing()
			}
		default:
			var cmd tea.Cmd
			m.textInput, cmd = m.textInput.Update(msg)
			return m, cmd
		}

	case CStepConfirm:
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
				m.step = CStepClipping
				return m, m.startClipping()
			} else {
				m.backToMenu = true
				return m, tea.Quit
			}
		case "y", "Y":
			m.step = CStepClipping
			return m, m.startClipping()
		case "n", "N":
			m.backToMenu = true
			return m, tea.Quit
		}

	case CStepComplete:
		switch msg.String() {
		case "enter", "q":
			return m, tea.Quit
		case "a":
			m.backToMenu = true
			return m, tea.Quit
		}

	case CStepError:
		switch msg.String() {
		case "enter", "r":
			m.backToMenu = true
			return m, tea.Quit
		case "q":
			return m, tea.Quit
		}
	}

	return m, nil
}

// goBack navigates to the previous step
func (m ClipModel) goBack() (tea.Model, tea.Cmd) {
	switch m.step {
	case CStepSelectProvider:
		m.backToMenu = true
		return m, tea.Quit
	case CStepEnterDescription:
		if len(m.availableProviders) > 1 {
			m.step = CStepSelectProvider
		} else {
			m.backToMenu = true
			return m, tea.Quit
		}
	case CStepConfirm:
		m.step = CStepEnterDescription
		m.textInput.Focus()
	}
	return m, nil
}

// clipProgressChan holds the current clip progress channel
var clipProgressChan chan clipProgressMsg

// waitForClipProgress waits for the next clip progress message
func waitForClipProgress(ch chan clipProgressMsg) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return nil
		}
		return msg
	}
}

// startParsing begins the AI parsing process
func (m ClipModel) startParsing() tea.Cmd {
	progressChan := make(chan clipProgressMsg, 100)
	resultChan := make(chan clipParseResultMsg, 1)

	// Store channel for continued listening
	clipProgressChan = progressChan

	provider := m.selectedProvider
	description := m.clipDescription
	duration := m.videoInfo.Duration

	// Start the parsing goroutine
	go func() {
		defer close(progressChan)

		parser, err := ai.NewParserWithProvider(provider)
		if err != nil {
			resultChan <- clipParseResultMsg{err: err}
			return
		}

		onProgress := func(update ai.ParserProgressUpdate) {
			select {
			case progressChan <- clipProgressMsg{
				status:       update.Status,
				provider:     update.Provider,
				model:        update.Model,
				message:      update.Message,
				detail:       update.Detail,
				requestInfo:  update.RequestInfo,
				responseInfo: update.ResponseInfo,
			}:
			default:
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		result, err := parser.ParseClipRequestWithProgress(ctx, description, duration, onProgress)
		resultChan <- clipParseResultMsg{result: result, err: err}
	}()

	// Return commands to listen for both progress and result
	return tea.Batch(
		waitForClipProgress(progressChan),
		func() tea.Msg {
			return <-resultChan
		},
	)
}

// startClipping begins the video clipping process
func (m ClipModel) startClipping() tea.Cmd {
	return func() tea.Msg {
		params := video.ClipParams{
			InputPath:  m.videoPath,
			StartTime:  m.clipRequest.StartTime,
			EndTime:    m.clipRequest.EndTime,
			OutputPath: m.outputPath,
		}

		err := video.ClipVideo(params)
		if err != nil {
			return clipCompleteMsg{err: err}
		}

		// Get output file info
		info, _ := os.Stat(m.outputPath)
		var size int64
		if info != nil {
			size = info.Size()
		}

		return clipCompleteMsg{outputPath: m.outputPath, outputSize: size}
	}
}

// View renders the UI
func (m ClipModel) View() string {
	if m.quitting {
		return MutedStyle.Render("Goodbye!\n")
	}

	var b strings.Builder

	// Compact header
	b.WriteString(lipgloss.NewStyle().
		Foreground(ColorCapybara).
		Bold(true).
		Render("CAPYCUT - Video Clipper") + "\n\n")

	// Main content based on step
	switch m.step {
	case CStepLoadingVideo:
		b.WriteString(m.renderLoading("Loading video information..."))
	case CStepSelectProvider:
		b.WriteString(m.renderProviderSelection())
	case CStepEnterDescription:
		b.WriteString(m.renderDescriptionInput())
	case CStepParsing:
		b.WriteString(m.renderParsing())
	case CStepConfirm:
		b.WriteString(m.renderConfirmation())
	case CStepClipping:
		b.WriteString(m.renderClipping())
	case CStepComplete:
		b.WriteString(m.renderComplete())
	case CStepError:
		b.WriteString(m.renderError())
	}

	// Help footer
	b.WriteString("\n")
	b.WriteString(m.renderHelp())

	return b.String()
}

// renderLoading renders a loading state
func (m ClipModel) renderLoading(message string) string {
	return BoxStyle.Render(
		m.spinner.View() + " " + BodyStyle.Render(message),
	)
}

// renderProviderSelection renders the provider selection
func (m ClipModel) renderProviderSelection() string {
	title := TitleStyle.Render("Select AI Provider")

	// Video info
	videoInfo := MutedStyle.Render(fmt.Sprintf("Video: %s | Duration: %s",
		m.videoInfo.Filename,
		video.FormatDuration(m.videoInfo.Duration)))

	var items strings.Builder
	for i, opt := range m.availableProviders {
		cursor := "  "
		style := BodyStyle
		if i == m.providerIndex {
			cursor = "> "
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		}
		items.WriteString(style.Render(cursor+opt.name) +
			MutedStyle.Render(" - "+opt.desc) + "\n")
	}

	return BoxStyle.Render(title + "\n" + videoInfo + "\n\n" + items.String())
}

// renderDescriptionInput renders the description input
func (m ClipModel) renderDescriptionInput() string {
	title := TitleStyle.Render("What would you like to clip?")

	// Video info
	videoInfo := MutedStyle.Render(fmt.Sprintf("Video: %s | Duration: %s",
		m.videoInfo.Filename,
		video.FormatDuration(m.videoInfo.Duration)))

	examples := MutedStyle.Render(`Examples:
  "from 3 minutes to 5 minutes 30 seconds"
  "first 2 minutes"
  "start at 1:23, end at 4:56"
  "last 45 seconds"`)

	return BoxStyle.Render(
		title + "\n" +
			videoInfo + "\n\n" +
			examples + "\n\n" +
			m.textInput.View(),
	)
}

// renderParsing renders the parsing progress with AI transparency
func (m ClipModel) renderParsing() string {
	title := TitleStyle.Render("Understanding your request...")

	var content strings.Builder

	// Simple status with spinner
	content.WriteString(m.spinner.View() + " ")
	if m.aiMessage != "" {
		content.WriteString(BodyStyle.Render(m.aiMessage))
	} else {
		content.WriteString(BodyStyle.Render("Thinking..."))
	}
	content.WriteString("\n\n")

	// AI activity feed (simple)
	feedStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1).
		Width(m.width - 12)
	content.WriteString(feedStyle.Render(m.aiFeed.Render()))

	return BoxStyle.Width(m.width - 4).Render(title + "\n\n" + content.String())
}

// renderConfirmation renders the confirmation screen
func (m ClipModel) renderConfirmation() string {
	title := TitleStyle.Render("Ready to Clip!")

	// Show AI feed summary
	feedSummary := SubtitleStyle.Render("AI Analysis Complete")

	// Calculate clip duration
	clipDuration, _ := video.CalculateClipDuration(m.clipRequest.StartTime, m.clipRequest.EndTime)

	// Summary
	summary := fmt.Sprintf(`Input:    %s
Start:    %s
End:      %s
Duration: %s
Output:   %s`,
		filepath.Base(m.videoPath),
		m.clipRequest.StartTime,
		m.clipRequest.EndTime,
		video.FormatDuration(clipDuration),
		filepath.Base(m.outputPath),
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
		yesStyle.Render("Yes, clip it!"),
		"  ",
		noStyle.Render("Cancel"),
	)

	return BoxStyle.Render(title + "\n" + feedSummary + "\n\n" + summaryBox + "\n\n" + buttons)
}

// renderClipping renders the clipping progress
func (m ClipModel) renderClipping() string {
	title := TitleStyle.Render("Clipping Video...")

	content := m.spinner.View() + " " + BodyStyle.Render("Processing with ffmpeg...")

	return BoxStyle.Render(title + "\n\n" + content)
}

// renderComplete renders the completion screen
func (m ClipModel) renderComplete() string {
	title := SuccessStyle.Render("Clip Complete!")

	elapsed := time.Since(m.startTime)

	summary := fmt.Sprintf(`Output:   %s
Size:     %s
Time:     %s`,
		m.outputPath,
		formatClipFileSize(m.outputSize),
		formatDuration(elapsed),
	)

	summaryBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorSuccess).
		Padding(1, 2).
		Render(summary)

	hint := MutedStyle.Render("\n[a] Another clip  [q] Quit")

	return BoxStyle.Render(title + "\n\n" + summaryBox + hint)
}

// renderError renders the error screen
func (m ClipModel) renderError() string {
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
func (m ClipModel) renderHelp() string {
	var keys []string

	switch m.step {
	case CStepSelectProvider:
		keys = append(keys, "j/k", "Navigate")
		keys = append(keys, "enter", "Select")
	case CStepEnterDescription:
		keys = append(keys, "enter", "Submit")
	case CStepConfirm:
		keys = append(keys, "y", "Yes")
		keys = append(keys, "n", "No")
	}

	if m.step != CStepParsing && m.step != CStepClipping && m.step != CStepComplete && m.step != CStepError {
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

// Helper function
func formatClipFileSize(bytes int64) string {
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

// convertParserStatusToFeedType converts ai.ParserProgressStatus to AIFeedMessageType
func convertParserStatusToFeedType(status ai.ParserProgressStatus) AIFeedMessageType {
	switch status {
	case ai.ParserStatusSendingRequest:
		return MsgTypeRequest
	case ai.ParserStatusParsingResponse, ai.ParserStatusComplete:
		return MsgTypeResponse
	case ai.ParserStatusWaitingResponse:
		return MsgTypeThinking
	case ai.ParserStatusError:
		return MsgTypeError
	default:
		return MsgTypeStatus
	}
}

// Getter methods for external access
func (m ClipModel) IsQuitting() bool { return m.quitting }
func (m ClipModel) BackToMenu() bool { return m.backToMenu }
func (m ClipModel) HasError() bool   { return m.step == CStepError }
func (m ClipModel) GetError() string { return m.errorMessage }
func (m ClipModel) IsComplete() bool { return m.step == CStepComplete }

// RunClipUI runs the clip UI and returns the result
func RunClipUI(videoPath string) (continueApp bool, err error) {
	model := NewClipModel(videoPath)
	p := tea.NewProgram(model, tea.WithAltScreen())

	finalModel, err := p.Run()
	if err != nil {
		return false, err
	}

	m := finalModel.(ClipModel)
	if m.BackToMenu() {
		return true, nil
	}
	if m.HasError() {
		return false, fmt.Errorf("%s", m.GetError())
	}
	return false, nil
}
