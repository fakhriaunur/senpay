// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, screen transitions.
// No business logic.
package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// settingsScreen is the settings screen model.
type settingsScreen struct {
	session    *Session
	focusIndex int // 0=language toggle, 1=save button
}

// newSettingsScreen creates a new settings screen.
func newSettingsScreen(session *Session) *settingsScreen {
	return &settingsScreen{
		session:    session,
		focusIndex: 0,
	}
}

func (s *settingsScreen) Init() tea.Cmd {
	return nil
}

func (s *settingsScreen) Update(msg tea.Msg) (*settingsScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return s, tea.Quit
		}

		switch msg.String() {
		case "tab", "down":
			s.focusIndex = (s.focusIndex + 1) % 2
			return s, nil

		case "shift+tab", "up":
			s.focusIndex = (s.focusIndex - 1 + 2) % 2
			return s, nil

		case "left", "right":
			if s.focusIndex == 0 {
				// Toggle language.
				if s.session.Lang() == "id" {
					s.session.SetLang("en")
				} else {
					s.session.SetLang("id")
				}
				return s, nil
			}
			return s, nil

		case "enter":
			if s.focusIndex == 1 {
				// Save and return to dashboard.
				return s, func() tea.Msg {
					return navigateToDashboardMsg{}
				}
			}
			return s, nil

		case "esc":
			return s, func() tea.Msg {
				return navigateToDashboardMsg{}
			}
		}
	}

	return s, nil
}

// navigateToSettingsMsg signals parent to navigate to settings.
type navigateToSettingsMsg struct{}

func (s *settingsScreen) View() string {
	var b strings.Builder
	lang := s.session.Lang()

	b.WriteString(TitleStyle.Render(T("settings_title", lang)))
	b.WriteString("\n\n")

	// Language selection.
	b.WriteString(InputPromptStyle.Render(T("settings_language", lang)))
	b.WriteString("\n")

	langOptions := []string{T("settings_lang_id", lang), T("settings_lang_en", lang)}
	selectedLang := 0
	if s.session.Lang() == "en" {
		selectedLang = 1
	}

	for i, opt := range langOptions {
		var line string
		if i == selectedLang {
			line = "◉ " + opt
			if s.focusIndex == 0 {
				line = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color(colorPrimary)).
					Render(line)
			} else {
				line = lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPrimary)).
					Render(line)
			}
		} else {
			line = "○ " + opt
			line = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorSecondary)).
				Render(line)
		}
		b.WriteString("  " + line + "\n")
	}

	b.WriteString("\n")

	// Save button.
	if s.focusIndex == 1 {
		b.WriteString(FocusedButtonStyle.Render(T("settings_save", lang)))
	} else {
		b.WriteString(ButtonStyle.Render(T("settings_save", lang)))
	}
	b.WriteString("\n\n")

	// Help text.
	b.WriteString(HelpStyle.Render(T("settings_help", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}
