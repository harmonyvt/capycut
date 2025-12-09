// Package tui provides a beautiful terminal UI for CapyCut using Charm libraries
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Color palette - Catppuccin Mocha inspired with some custom touches
var (
	// Primary colors
	ColorPrimary   = lipgloss.AdaptiveColor{Light: "#7C3AED", Dark: "#A78BFA"} // Violet
	ColorSecondary = lipgloss.AdaptiveColor{Light: "#0EA5E9", Dark: "#38BDF8"} // Sky blue
	ColorAccent    = lipgloss.AdaptiveColor{Light: "#F59E0B", Dark: "#FBBF24"} // Amber

	// Semantic colors
	ColorSuccess = lipgloss.AdaptiveColor{Light: "#10B981", Dark: "#34D399"} // Emerald
	ColorWarning = lipgloss.AdaptiveColor{Light: "#F59E0B", Dark: "#FBBF24"} // Amber
	ColorError   = lipgloss.AdaptiveColor{Light: "#EF4444", Dark: "#F87171"} // Red
	ColorInfo    = lipgloss.AdaptiveColor{Light: "#6366F1", Dark: "#818CF8"} // Indigo

	// Neutral colors
	ColorText       = lipgloss.AdaptiveColor{Light: "#1E293B", Dark: "#F1F5F9"}
	ColorSubtle     = lipgloss.AdaptiveColor{Light: "#64748B", Dark: "#94A3B8"}
	ColorMuted      = lipgloss.AdaptiveColor{Light: "#94A3B8", Dark: "#64748B"}
	ColorBorder     = lipgloss.AdaptiveColor{Light: "#CBD5E1", Dark: "#334155"}
	ColorBackground = lipgloss.AdaptiveColor{Light: "#F8FAFC", Dark: "#0F172A"}

	// Special colors
	ColorCapybara  = lipgloss.AdaptiveColor{Light: "#D97706", Dark: "#F59E0B"} // Capybara brown/amber
	ColorGradient1 = lipgloss.AdaptiveColor{Light: "#8B5CF6", Dark: "#A78BFA"}
	ColorGradient2 = lipgloss.AdaptiveColor{Light: "#EC4899", Dark: "#F472B6"}
)

// Base styles
var (
	// Text styles
	TitleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorPrimary).
			MarginBottom(1)

	SubtitleStyle = lipgloss.NewStyle().
			Foreground(ColorSecondary).
			MarginBottom(1)

	BodyStyle = lipgloss.NewStyle().
			Foreground(ColorText)

	MutedStyle = lipgloss.NewStyle().
			Foreground(ColorMuted)

	// Status styles
	SuccessStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorSuccess)

	ErrorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(ColorError)

	WarningStyle = lipgloss.NewStyle().
			Foreground(ColorWarning)

	InfoStyle = lipgloss.NewStyle().
			Foreground(ColorInfo)

	// Component styles
	BoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorBorder).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	FocusedBoxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(ColorPrimary).
			Padding(1, 2).
			MarginTop(1).
			MarginBottom(1)

	// Badge styles
	BadgeStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorPrimary).
			Foreground(lipgloss.Color("#FFFFFF"))

	BadgeSuccessStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(ColorSuccess).
				Foreground(lipgloss.Color("#FFFFFF"))

	BadgeWarningStyle = lipgloss.NewStyle().
				Padding(0, 1).
				Background(ColorWarning).
				Foreground(lipgloss.Color("#000000"))

	BadgeErrorStyle = lipgloss.NewStyle().
			Padding(0, 1).
			Background(ColorError).
			Foreground(lipgloss.Color("#FFFFFF"))
)

// Application header with ASCII art
var CapybaraASCII = `
   ___   _   _____   _____ _   _ _____ 
  / __| /_\ | _ \ \ / / __| | | |_   _|
 | (__ / _ \|  _/\ V /| _|| |_| | | |  
  \___/_/ \_\_|   |_| |___|\___/  |_|  
`

var CapybaraMini = `
     /\_/\  
    ( o.o ) 
     > ^ <
`

// GetCapybaraHeader returns the styled header
func GetCapybaraHeader() string {
	return lipgloss.NewStyle().
		Foreground(ColorCapybara).
		Bold(true).
		Render(CapybaraASCII)
}

// WizardStep represents a step in the wizard UI
type WizardStep struct {
	Title       string
	Description string
	Icon        string
	Status      StepStatus
}

// StepStatus represents the status of a wizard step
type StepStatus int

const (
	StepPending StepStatus = iota
	StepActive
	StepCompleted
	StepError
)

// StepIndicator renders a step indicator for the wizard
func StepIndicator(steps []WizardStep, currentStep int, width int) string {
	var result string

	// Header line
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		Width(width).
		Align(lipgloss.Center)

	result += headerStyle.Render("Image to Markdown Transcription") + "\n\n"

	// Steps indicator
	for i, step := range steps {
		var icon string
		var style lipgloss.Style

		switch step.Status {
		case StepCompleted:
			icon = "[x]"
			style = lipgloss.NewStyle().Foreground(ColorSuccess)
		case StepActive:
			icon = "[>]"
			style = lipgloss.NewStyle().Foreground(ColorPrimary).Bold(true)
		case StepError:
			icon = "[!]"
			style = lipgloss.NewStyle().Foreground(ColorError)
		default:
			icon = "[ ]"
			style = lipgloss.NewStyle().Foreground(ColorMuted)
		}

		stepText := style.Render(icon + " " + step.Title)

		// Add connector line
		if i < len(steps)-1 {
			if step.Status == StepCompleted || step.Status == StepActive {
				stepText += lipgloss.NewStyle().Foreground(ColorSuccess).Render(" ---")
			} else {
				stepText += lipgloss.NewStyle().Foreground(ColorMuted).Render(" ---")
			}
		}

		result += stepText
	}

	result += "\n"
	return result
}

// ProgressBar creates a beautiful progress bar
func ProgressBar(current, total int, width int) string {
	if total == 0 {
		total = 1
	}

	percentage := float64(current) / float64(total)
	filled := int(percentage * float64(width))
	if filled > width {
		filled = width
	}

	// Use gradient-like blocks
	filledChar := lipgloss.NewStyle().Foreground(ColorPrimary).Render("█")
	emptyChar := lipgloss.NewStyle().Foreground(ColorBorder).Render("░")

	bar := ""
	for i := 0; i < width; i++ {
		if i < filled {
			bar += filledChar
		} else {
			bar += emptyChar
		}
	}

	// Percentage text
	percentText := lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Render(fmt.Sprintf(" %3d%%", int(percentage*100)))

	return bar + percentText
}

// SpinnerFrames contains frames for the custom spinner animation
var SpinnerFrames = []string{
	"( o.o )  ",
	"( o.o ) >",
	"( o.o )>>",
	"( o.o )> ",
	"( o.o )  ",
	"( o.o)   ",
	"(o.o )   ",
	"( o.o )  ",
}

// Card renders a card component
func Card(title, content string, width int) string {
	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorPrimary).
		MarginBottom(1)

	contentStyle := lipgloss.NewStyle().
		Foreground(ColorText)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(ColorBorder).
		Padding(1, 2).
		Width(width)

	innerContent := titleStyle.Render(title) + "\n" + contentStyle.Render(content)
	return cardStyle.Render(innerContent)
}

// StatusCard renders a status card with an icon
func StatusCard(icon, title, subtitle string, status StepStatus, width int) string {
	var borderColor lipgloss.AdaptiveColor
	var iconStyle lipgloss.Style

	switch status {
	case StepCompleted:
		borderColor = ColorSuccess
		iconStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	case StepActive:
		borderColor = ColorPrimary
		iconStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
	case StepError:
		borderColor = ColorError
		iconStyle = lipgloss.NewStyle().Foreground(ColorError)
	default:
		borderColor = ColorBorder
		iconStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}

	titleStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(ColorText)

	subtitleStyle := lipgloss.NewStyle().
		Foreground(ColorSubtle)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2).
		Width(width)

	content := iconStyle.Render(icon) + " " + titleStyle.Render(title)
	if subtitle != "" {
		content += "\n   " + subtitleStyle.Render(subtitle)
	}

	return cardStyle.Render(content)
}

// KeyHelp renders keyboard shortcut help
func KeyHelp(keys map[string]string) string {
	helpStyle := lipgloss.NewStyle().
		Foreground(ColorMuted).
		MarginTop(1)

	keyStyle := lipgloss.NewStyle().
		Foreground(ColorSubtle).
		Bold(true)

	descStyle := lipgloss.NewStyle().
		Foreground(ColorMuted)

	var parts []string
	for key, desc := range keys {
		parts = append(parts, keyStyle.Render(key)+descStyle.Render(" "+desc))
	}

	// Join with separator
	sep := lipgloss.NewStyle().Foreground(ColorBorder).Render(" | ")
	return helpStyle.Render(joinStrings(parts, sep))
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}

// AIAgentStatus represents the status of an AI agent task
type AIAgentStatus int

const (
	AgentIdle AIAgentStatus = iota
	AgentConnecting
	AgentProcessing
	AgentWaitingResponse
	AgentParsing
	AgentComplete
	AgentError
)

// AIAgentInfo holds information about an AI agent's current activity
type AIAgentInfo struct {
	Name        string        // Agent name (e.g., "Gemini 3 Pro", "Local LLM", "Claude")
	Provider    string        // Provider type (e.g., "gemini", "local", "azure_anthropic")
	Status      AIAgentStatus // Current status
	Task        string        // What the agent is doing
	Detail      string        // Additional detail (e.g., "Processing batch 2/5")
	Progress    float64       // Progress 0.0-1.0
	TokensUsed  int           // Tokens consumed
	ElapsedTime string        // Time elapsed
}

// AgentStatusText returns a human-readable status string
func (s AIAgentStatus) String() string {
	switch s {
	case AgentIdle:
		return "Idle"
	case AgentConnecting:
		return "Connecting"
	case AgentProcessing:
		return "Processing"
	case AgentWaitingResponse:
		return "Waiting for response"
	case AgentParsing:
		return "Parsing response"
	case AgentComplete:
		return "Complete"
	case AgentError:
		return "Error"
	default:
		return "Unknown"
	}
}

// AgentStatusIcon returns an icon for the status
func (s AIAgentStatus) Icon() string {
	switch s {
	case AgentIdle:
		return "[ ]"
	case AgentConnecting:
		return "[~]"
	case AgentProcessing:
		return "[>]"
	case AgentWaitingResponse:
		return "[.]"
	case AgentParsing:
		return "[*]"
	case AgentComplete:
		return "[x]"
	case AgentError:
		return "[!]"
	default:
		return "[?]"
	}
}

// RenderAgentCard renders a card showing AI agent status
func RenderAgentCard(agent AIAgentInfo, width int) string {
	var borderColor lipgloss.AdaptiveColor
	var statusStyle lipgloss.Style

	switch agent.Status {
	case AgentComplete:
		borderColor = ColorSuccess
		statusStyle = lipgloss.NewStyle().Foreground(ColorSuccess)
	case AgentError:
		borderColor = ColorError
		statusStyle = lipgloss.NewStyle().Foreground(ColorError)
	case AgentProcessing, AgentWaitingResponse, AgentParsing:
		borderColor = ColorPrimary
		statusStyle = lipgloss.NewStyle().Foreground(ColorPrimary)
	default:
		borderColor = ColorBorder
		statusStyle = lipgloss.NewStyle().Foreground(ColorMuted)
	}

	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(ColorText)
	subtitleStyle := lipgloss.NewStyle().Foreground(ColorSubtle)
	detailStyle := lipgloss.NewStyle().Foreground(ColorMuted)

	cardStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(0, 2).
		Width(width)

	// Build content
	var lines []string

	// Header: Icon + Name + Status
	header := statusStyle.Render(agent.Status.Icon()) + " " +
		titleStyle.Render(agent.Name) + " " +
		subtitleStyle.Render("("+agent.Provider+")")
	lines = append(lines, header)

	// Task description
	if agent.Task != "" {
		lines = append(lines, "  "+statusStyle.Render(agent.Status.String())+": "+agent.Task)
	}

	// Detail (e.g., batch progress)
	if agent.Detail != "" {
		lines = append(lines, "  "+detailStyle.Render(agent.Detail))
	}

	// Progress bar (if processing)
	if agent.Status == AgentProcessing || agent.Status == AgentWaitingResponse {
		if agent.Progress > 0 {
			progressStr := ProgressBar(int(agent.Progress*100), 100, width-8)
			lines = append(lines, "  "+progressStr)
		}
	}

	// Stats line
	var stats []string
	if agent.TokensUsed > 0 {
		stats = append(stats, fmt.Sprintf("Tokens: %d", agent.TokensUsed))
	}
	if agent.ElapsedTime != "" {
		stats = append(stats, "Time: "+agent.ElapsedTime)
	}
	if len(stats) > 0 {
		lines = append(lines, "  "+detailStyle.Render(joinStrings(stats, " | ")))
	}

	content := joinStrings(lines, "\n")
	return cardStyle.Render(content)
}

// RenderMultiAgentStatus renders status for multiple agents (for two-stage pipeline)
func RenderMultiAgentStatus(agents []AIAgentInfo, width int) string {
	var sections []string

	for _, agent := range agents {
		sections = append(sections, RenderAgentCard(agent, width))
	}

	return joinStrings(sections, "\n")
}

// RenderProcessingStatus renders a simple processing status line
func RenderProcessingStatus(spinner, message, detail string) string {
	var sb strings.Builder

	sb.WriteString(spinner + " ")
	sb.WriteString(BodyStyle.Render(message))

	if detail != "" {
		sb.WriteString("\n  ")
		sb.WriteString(MutedStyle.Render(detail))
	}

	return sb.String()
}
