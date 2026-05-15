// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: This is the Imperative Shell — it manages user interaction,
// HTTP calls, and screen transitions. No business logic here.
package tui

import (
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/lipgloss"
)

// Color palette.
const (
	colorPrimary   = "#00BFFF" // Deep sky blue
	colorSecondary = "#7A7A7A" // Gray
	colorSuccess   = "#00FF7F" // Spring green
	colorError     = "#FF4444" // Red
	colorWhite     = "#FFFFFF"
	colorDarkBg    = "#1A1A2E"
	colorCardBg    = "#16213E"
	colorHighlight = "#0F3460"
	colorMuted     = "#555555"
	colorSenpai    = "#FFD700" // Gold for senpai tips
)

// Application styles.
var (
	// AppStyle is the main application background style.
	AppStyle = lipgloss.NewStyle().
		Background(lipgloss.Color(colorDarkBg)).
		Width(80).
		Align(lipgloss.Center).
		PaddingTop(1)

	// TitleStyle renders the app title.
	TitleStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary)).
		Padding(0, 1).
		Align(lipgloss.Center).
		Width(40)

	// SubtitleStyle renders subtitle text.
	SubtitleStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSecondary)).
		Align(lipgloss.Center).
		Width(40)

	// InputPromptStyle renders the label for an input field.
	InputPromptStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorWhite)).
		Bold(true).
		MarginTop(1)

	// InputStyle renders a text input field with border.
	InputStyle = func() lipgloss.Style {
		s := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorSecondary)).
			Padding(0, 1).
			Width(30)
		return s
	}

	// FocusedInputStyle renders a focused text input field.
	FocusedInputStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color(colorPrimary)).
			Padding(0, 1).
			Width(30)

	// ButtonStyle renders a clickable button.
	ButtonStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorPrimary)).
		Padding(0, 3).
		MarginTop(1).
		Align(lipgloss.Center).
		Width(20)

	// FocusedButtonStyle renders a focused button.
	FocusedButtonStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorHighlight)).
		Padding(0, 3).
		MarginTop(1).
		Align(lipgloss.Center).
		Width(20)

	// ErrorStyle renders error messages.
	ErrorStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorError)).
		Bold(true).
		Width(40).
		Align(lipgloss.Center)

	// SuccessStyle renders success messages.
	SuccessStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSuccess)).
		Bold(true).
		Width(40).
		Align(lipgloss.Center)

	// BalanceStyle renders the balance amount.
	BalanceStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorSuccess)).
		Width(40).
		Align(lipgloss.Center).
		MarginTop(1)

	// BalanceLabelStyle renders the balance label.
	BalanceLabelStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSecondary)).
		Width(40).
		Align(lipgloss.Center)

	// GreetingStyle renders the user greeting.
	GreetingStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Width(40).
		Align(lipgloss.Center).
		MarginTop(1)

	// QuickActionStyle renders a quick action button on dashboard.
	QuickActionStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorHighlight)).
		Padding(0, 2).
		Margin(0, 1).
		Align(lipgloss.Center).
		Width(18)

	// FocusedQuickActionStyle renders a focused quick action button.
	FocusedQuickActionStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorPrimary)).
		Padding(0, 2).
		Margin(0, 1).
		Align(lipgloss.Center).
		Width(18)

	// SenpaiTipStyle renders the senpai tip.
	SenpaiTipStyle = lipgloss.NewStyle().
		Italic(true).
		Foreground(lipgloss.Color(colorSenpai)).
		Width(60).
		Align(lipgloss.Center).
		MarginTop(2).
		Padding(0, 2)

	// HelpStyle renders help text at the bottom.
	HelpStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Width(60).
		Align(lipgloss.Center).
		MarginTop(1)

	// NavStyle renders navigation tabs.
	NavStyle = lipgloss.NewStyle().
		Width(80).
		Align(lipgloss.Center).
		PaddingTop(1).
		PaddingBottom(1)

	// NavTabStyle renders a navigation tab.
	NavTabStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSecondary)).
		Padding(0, 1).
		Margin(0, 0)

	// NavTabActiveStyle renders the active navigation tab.
	NavTabActiveStyle = lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary)).
		Underline(true).
		Padding(0, 1).
		Margin(0, 0)

	// NavTabDisabledStyle renders a disabled navigation tab.
	NavTabDisabledStyle = lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorMuted)).
		Padding(0, 1).
		Margin(0, 0)
)

// NewTextInput creates a new text input with the given placeholder and focus state.
func NewTextInput(placeholder string, focus bool, password bool) textinput.Model {
	ti := textinput.New()
	ti.Placeholder = placeholder
	ti.Width = 28
	ti.TextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite))
	ti.PlaceholderStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted))
	if password {
		ti.EchoMode = textinput.EchoPassword
		ti.EchoCharacter = '•'
	}
	if focus {
		ti.Focus()
	} else {
		ti.Blur()
	}
	return ti
}
