// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// screenID identifies which screen is active.
type screenID int

const (
	screenLogin screenID = iota
	screenDashboard
	screenTransfer
	screenTransferSuccess
	screenHistory
	screenDetail
)

// Model is the top-level Bubble Tea model that manages screens.
type Model struct {
	session     *Session
	current     screenID
	login       *loginScreen
	dashboard   *dashboardScreen
	transfer    *transferScreen
	history     *historyScreen
	detail      *detailScreen
	quitting    bool
	windowWidth  int
	windowHeight int
}

// New creates a new TUI model.
func New() *Model {
	session := NewSession()
	return &Model{
		session: session,
		current: screenLogin,
		login:   newLoginScreen(session),
	}
}

// Init initializes the TUI.
func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		m.login.Init(),
		tea.EnterAltScreen,
	)
}

// Update handles messages and updates the model.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	// Handle window resize.
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.windowWidth = msg.Width
		m.windowHeight = msg.Height
	}

	// Handle global keys.
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Ctrl+C handling with confirmation.
		if msg.String() == "ctrl+c" {
			if m.quitting {
				return m, tea.Quit
			}
			m.quitting = true
			return m, nil
		}

		// If we're in "quit confirmation" mode, any key other than ctrl+c
		// dismisses the confirmation.
		if m.quitting {
			m.quitting = false
			return m, nil
		}
	}

	// Handle navigation messages globally.
	switch msg := msg.(type) {
	case navigateToDashboardMsg:
		m.transitionToDashboard()
		return m, m.dashboard.Init()

	case viewDetailMsg:
		m.current = screenDetail
		m.detail = newDetailScreen(m.session, msg.tx)
		return m, nil

	case navigateToHistoryMsg:
		m.current = screenHistory
		// Refresh history when coming back from detail.
		m.history = newHistoryScreen(m.session)
		return m, m.history.Init()
	}

	// Delegate to the active screen.
	switch m.current {
	case screenLogin:
		return m.updateLogin(msg)
	case screenDashboard:
		return m.updateDashboard(msg)
	case screenTransfer:
		return m.updateTransfer(msg)
	case screenHistory:
		return m.updateHistory(msg)
	case screenDetail:
		return m.updateDetail(msg)
	}

	return m, nil
}

// updateLogin handles messages for the login screen.
func (m *Model) updateLogin(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.login, cmd = m.login.Update(msg)

	// Check for login completion.
	if _, ok := msg.(loginCompleteMsg); ok {
		m.transitionToDashboard()
		return m, m.dashboard.Init()
	}

	return m, cmd
}

// updateDashboard handles messages for the dashboard screen.
func (m *Model) updateDashboard(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.dashboard, cmd = m.dashboard.Update(msg)

	// Check for quick action selections.
	switch msg := msg.(type) {
	case quickActionSelectedMsg:
		switch msg.actionID {
		case ActionTransfer:
			m.current = screenTransfer
			m.transfer = newTransferScreen(m.session)
			return m, m.transfer.Init()
		case ActionHistory:
			m.current = screenHistory
			m.history = newHistoryScreen(m.session)
			return m, m.history.Init()
		}
	}

	return m, cmd
}

// updateTransfer handles messages for the transfer screen.
func (m *Model) updateTransfer(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.transfer, cmd = m.transfer.Update(msg)
	return m, cmd
}

// updateHistory handles messages for the history screen.
func (m *Model) updateHistory(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.history, cmd = m.history.Update(msg)
	return m, cmd
}

// updateDetail handles messages for the detail screen.
func (m *Model) updateDetail(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.detail, cmd = m.detail.Update(msg)
	return m, cmd
}

// transitionToDashboard switches from login to dashboard screen.
func (m *Model) transitionToDashboard() {
	m.current = screenDashboard
	m.dashboard = newDashboardScreen(m.session)
}

// View renders the current screen.
func (m *Model) View() string {
	// If in quit confirmation, overlay confirmation dialog.
	if m.quitting {
		return m.renderQuitConfirmation()
	}

	switch m.current {
	case screenLogin:
		return m.login.View()
	case screenDashboard:
		return m.dashboard.View()
	case screenTransfer:
		return m.transfer.View()
	case screenHistory:
		return m.history.View()
	case screenDetail:
		return m.detail.View()
	}
	return ""
}

// renderQuitConfirmation renders a quit confirmation dialog.
func (m *Model) renderQuitConfirmation() string {
	// Create a centered confirmation dialog.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(40).
		Align(lipgloss.Center)

	dialogContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError)).Render("Yakin ingin keluar?"),
		"",
		"Tekan Ctrl+C lagi untuk keluar",
		"atau tombol lain untuk batal",
	)

	return lipgloss.NewStyle().
		Width(80).
		Height(24).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogStyle.Render(dialogContent))
}

// Run starts the TUI application.
func Run() {
	m := New()
	p := tea.NewProgram(m, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
