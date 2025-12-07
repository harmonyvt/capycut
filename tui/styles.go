// Package tui provides a beautiful terminal UI for CapyCut using Charm libraries
package tui

import (
	"fmt"

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
