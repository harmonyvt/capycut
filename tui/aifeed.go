// Package tui provides a beautiful terminal UI for CapyCut using Charm libraries
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	"github.com/charmbracelet/lipgloss"
)

// AIFeedMessageType represents the type of AI feed message
type AIFeedMessageType string

const (
	// MsgTypeRequest indicates an outgoing AI request
	MsgTypeRequest AIFeedMessageType = "request"
	// MsgTypeResponse indicates an incoming AI response
	MsgTypeResponse AIFeedMessageType = "response"
	// MsgTypeStatus indicates a status update
	MsgTypeStatus AIFeedMessageType = "status"
	// MsgTypeThinking indicates AI is processing
	MsgTypeThinking AIFeedMessageType = "thinking"
	// MsgTypeError indicates an error occurred
	MsgTypeError AIFeedMessageType = "error"
	// MsgTypeComplete indicates successful completion
	MsgTypeComplete AIFeedMessageType = "complete"
)

// AIFeedMessage represents a single message in the AI transparency feed
type AIFeedMessage struct {
	// Timestamp when the message was created
	Timestamp time.Time

	// Type of message (request, response, status, etc.)
	Type AIFeedMessageType

	// Provider name (e.g., "Google Gemini", "Local LLM", "Azure Anthropic")
	Provider string

	// Model name being used
	Model string

	// Title is the main message text
	Title string

	// Details contains additional information
	Details []string

	// RequestInfo contains details about the AI request (for request type)
	RequestInfo *AIRequestInfo

	// ResponseInfo contains details about the AI response (for response type)
	ResponseInfo *AIResponseInfo
}

// AIRequestInfo contains transparency details about an AI request
type AIRequestInfo struct {
	// Endpoint URL (sanitized, no API keys)
	Endpoint string

	// Method (POST, GET, etc.)
	Method string

	// ContentType of the request
	ContentType string

	// DataSummary describes what data is being sent
	DataSummary string

	// ImageCount for image transcription requests
	ImageCount int

	// TotalDataSize in bytes
	TotalDataSize int64

	// PromptPreview shows first N characters of the prompt
	PromptPreview string

	// Parameters like temperature, max_tokens, etc.
	Parameters map[string]string
}

// AIResponseInfo contains transparency details about an AI response
type AIResponseInfo struct {
	// StatusCode HTTP status code
	StatusCode int

	// StatusText HTTP status text
	StatusText string

	// Latency time taken for the request
	Latency time.Duration

	// TokensInput consumed
	TokensInput int

	// TokensOutput generated
	TokensOutput int

	// TokensTotal consumed
	TokensTotal int

	// ContentPreview shows first N characters of response content
	ContentPreview string

	// ItemsProcessed (pages, clips, etc.)
	ItemsProcessed int

	// ErrorMessage if any
	ErrorMessage string
}

// AIFeed manages a list of AI transparency messages with viewport
type AIFeed struct {
	// Messages in the feed
	Messages []AIFeedMessage

	// Viewport for scrolling
	Viewport viewport.Model

	// Width of the feed display
	Width int

	// Height of the feed display
	Height int

	// ShowRequestDetails toggles detailed request info
	ShowRequestDetails bool

	// ShowResponseDetails toggles detailed response info
	ShowResponseDetails bool

	// MaxMessages limits the number of messages kept (0 = unlimited)
	MaxMessages int
}

// NewAIFeed creates a new AI feed with the given dimensions
func NewAIFeed(width, height int) *AIFeed {
	vp := viewport.New(width, height)
	vp.Style = lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(0, 1)

	return &AIFeed{
		Messages:            make([]AIFeedMessage, 0),
		Viewport:            vp,
		Width:               width,
		Height:              height,
		ShowRequestDetails:  true,
		ShowResponseDetails: true,
		MaxMessages:         100,
	}
}

// AddMessage adds a new message to the feed
func (f *AIFeed) AddMessage(msg AIFeedMessage) {
	if msg.Timestamp.IsZero() {
		msg.Timestamp = time.Now()
	}

	f.Messages = append(f.Messages, msg)

	// Trim old messages if needed
	if f.MaxMessages > 0 && len(f.Messages) > f.MaxMessages {
		f.Messages = f.Messages[len(f.Messages)-f.MaxMessages:]
	}

	// Update viewport content
	f.Viewport.SetContent(f.Render())
	f.Viewport.GotoBottom()
}

// AddRequest adds a request message to the feed
func (f *AIFeed) AddRequest(provider, model string, info *AIRequestInfo) {
	title := fmt.Sprintf("REQUEST to %s", provider)
	if model != "" {
		title += fmt.Sprintf(" (%s)", model)
	}

	f.AddMessage(AIFeedMessage{
		Timestamp:   time.Now(),
		Type:        MsgTypeRequest,
		Provider:    provider,
		Model:       model,
		Title:       title,
		RequestInfo: info,
	})
}

// AddResponse adds a response message to the feed
func (f *AIFeed) AddResponse(provider, model string, info *AIResponseInfo) {
	title := fmt.Sprintf("RESPONSE from %s", provider)
	if info != nil && info.Latency > 0 {
		title += fmt.Sprintf(" (%.1fs)", info.Latency.Seconds())
	}

	f.AddMessage(AIFeedMessage{
		Timestamp:    time.Now(),
		Type:         MsgTypeResponse,
		Provider:     provider,
		Model:        model,
		Title:        title,
		ResponseInfo: info,
	})
}

// AddStatus adds a status message to the feed
func (f *AIFeed) AddStatus(provider, model, title string, details ...string) {
	f.AddMessage(AIFeedMessage{
		Timestamp: time.Now(),
		Type:      MsgTypeStatus,
		Provider:  provider,
		Model:     model,
		Title:     title,
		Details:   details,
	})
}

// AddThinking adds a "thinking" message to the feed
func (f *AIFeed) AddThinking(provider, model, title string) {
	f.AddMessage(AIFeedMessage{
		Timestamp: time.Now(),
		Type:      MsgTypeThinking,
		Provider:  provider,
		Model:     model,
		Title:     title,
	})
}

// AddError adds an error message to the feed
func (f *AIFeed) AddError(provider, model, title, errorMsg string) {
	f.AddMessage(AIFeedMessage{
		Timestamp: time.Now(),
		Type:      MsgTypeError,
		Provider:  provider,
		Model:     model,
		Title:     title,
		ResponseInfo: &AIResponseInfo{
			ErrorMessage: errorMsg,
		},
	})
}

// AddComplete adds a completion message to the feed
func (f *AIFeed) AddComplete(provider, model, title string, details ...string) {
	f.AddMessage(AIFeedMessage{
		Timestamp: time.Now(),
		Type:      MsgTypeComplete,
		Provider:  provider,
		Model:     model,
		Title:     title,
		Details:   details,
	})
}

// SetSize updates the feed dimensions
func (f *AIFeed) SetSize(width, height int) {
	f.Width = width
	f.Height = height
	f.Viewport.Width = width
	f.Viewport.Height = height
	f.Viewport.SetContent(f.Render())
}

// Clear removes all messages from the feed
func (f *AIFeed) Clear() {
	f.Messages = make([]AIFeedMessage, 0)
	f.Viewport.SetContent(f.Render())
}

// View returns the viewport view for Bubble Tea
func (f *AIFeed) View() string {
	return f.Viewport.View()
}

// Render renders all messages to a string
func (f *AIFeed) Render() string {
	if len(f.Messages) == 0 {
		return MutedStyle.Render("  Waiting for AI...")
	}

	var lines []string
	for _, msg := range f.Messages {
		line := f.renderMessageSimple(msg)
		if line != "" {
			lines = append(lines, line)
		}
	}

	return strings.Join(lines, "\n")
}

// renderMessageSimple renders a single message in a very simple format
func (f *AIFeed) renderMessageSimple(msg AIFeedMessage) string {
	// Get icon and style based on message type
	icon, style := f.getMessageStyle(msg.Type)

	// Timestamp
	timestamp := msg.Timestamp.Format("15:04:05")
	timestampStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	// Build the message with optional metrics
	var suffix string

	// Add response metrics if available
	if msg.ResponseInfo != nil {
		var parts []string
		if msg.ResponseInfo.Latency > 0 {
			parts = append(parts, fmt.Sprintf("%.1fs", msg.ResponseInfo.Latency.Seconds()))
		}
		if msg.ResponseInfo.TokensTotal > 0 {
			parts = append(parts, fmt.Sprintf("%d tokens", msg.ResponseInfo.TokensTotal))
		}
		if msg.ResponseInfo.ItemsProcessed > 0 {
			parts = append(parts, fmt.Sprintf("%d pages", msg.ResponseInfo.ItemsProcessed))
		}
		if len(parts) > 0 {
			suffix = " " + MutedStyle.Render("("+strings.Join(parts, ", ")+")")
		}
		if msg.ResponseInfo.ErrorMessage != "" {
			suffix = " " + lipgloss.NewStyle().Foreground(ColorError).Render("- "+msg.ResponseInfo.ErrorMessage)
		}
	}

	// Add request info if available (for images)
	if msg.RequestInfo != nil && msg.ResponseInfo == nil {
		if msg.RequestInfo.ImageCount > 0 {
			suffix = " " + MutedStyle.Render(fmt.Sprintf("(%d images)", msg.RequestInfo.ImageCount))
		} else if msg.RequestInfo.DataSummary != "" {
			suffix = " " + MutedStyle.Render("("+msg.RequestInfo.DataSummary+")")
		}
	}

	return fmt.Sprintf("%s %s %s%s",
		timestampStyle.Render(timestamp),
		style.Render(icon),
		style.Render(msg.Title),
		suffix,
	)
}

// getMessageStyle returns icon and style for a message type
func (f *AIFeed) getMessageStyle(msgType AIFeedMessageType) (string, lipgloss.Style) {
	switch msgType {
	case MsgTypeRequest:
		return "[>]", lipgloss.NewStyle().Foreground(ColorSecondary)
	case MsgTypeResponse:
		return "[<]", lipgloss.NewStyle().Foreground(ColorSuccess)
	case MsgTypeThinking:
		return "[.]", lipgloss.NewStyle().Foreground(ColorAccent)
	case MsgTypeError:
		return "[!]", lipgloss.NewStyle().Foreground(ColorError)
	case MsgTypeComplete:
		return "[x]", lipgloss.NewStyle().Foreground(ColorSuccess)
	default:
		return "[-]", lipgloss.NewStyle().Foreground(ColorPrimary)
	}
}

// Helper functions

func truncateString(s string, maxLen int) string {
	// Remove newlines for preview
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")

	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatDataSize(bytes int64) string {
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
		return fmt.Sprintf("%d B", bytes)
	}
}

// RenderFeedTitle renders a title for the AI feed section
func RenderFeedTitle(title string) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)

	return titleStyle.Render(title)
}

// RenderFeedBox renders the feed in a styled box
func RenderFeedBox(feed *AIFeed, title string, width int) string {
	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Width(width).
		Padding(0, 1)

	titleStr := RenderFeedTitle(title)
	content := feed.Viewport.View()

	return titleStr + "\n" + boxStyle.Render(content)
}
