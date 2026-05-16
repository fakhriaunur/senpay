// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"os"
	"strings"

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
	screenTopup
	screenWithdraw
	screenSettings
)

// minimum terminal dimensions.
const (
	minWidth  = 80
	minHeight = 24
)

// DefaultTUIMinAmountSen is the minimum amount for top-up and withdraw in the TUI (Rp 100).
const DefaultTUIMinAmountSen = 10_000

// TUIPhoneMaxLength is the maximum phone length accepted by the TUI.
const TUIPhoneMaxLength = 15

// Model is the top-level Bubble Tea model that manages screens.
type Model struct {
	session      *Session
	current      screenID
	login        *loginScreen
	dashboard    *dashboardScreen
	transfer     *transferScreen
	history      *historyScreen
	detail       *detailScreen
	topup        *topupScreen
	withdraw     *withdrawScreen
	settings     *settingsScreen
	quitting     bool
	showingHelp  bool
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
		// ? key toggles help overlay (if not in quit confirmation).
		if msg.String() == "?" && !m.quitting {
			m.showingHelp = !m.showingHelp
			return m, nil
		}

		// If help overlay is showing, Esc dismisses it.
		if m.showingHelp {
			if msg.String() == "esc" || msg.String() == "?" {
				m.showingHelp = false
				return m, nil
			}
			// Other keys don't dismiss help in some designs, but
			// it's friendlier to dismiss on any key.
			m.showingHelp = false
			return m, nil
		}

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

	case navigateToSettingsMsg:
		m.current = screenSettings
		m.settings = newSettingsScreen(m.session)
		return m, nil
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
	case screenTopup:
		return m.updateTopup(msg)
	case screenWithdraw:
		return m.updateWithdraw(msg)
	case screenSettings:
		return m.updateSettings(msg)
	}

	return m, nil
}

// updateSettings handles messages for the settings screen.
func (m *Model) updateSettings(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.settings, cmd = m.settings.Update(msg)
	return m, cmd
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
		case ActionTopUp:
			m.current = screenTopup
			m.topup = newTopupScreen(m.session)
			return m, m.topup.Init()
		case ActionWithdraw:
			m.current = screenWithdraw
			m.withdraw = newWithdrawScreen(m.session)
			return m, m.withdraw.Init()
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

// updateTopup handles messages for the top-up screen.
func (m *Model) updateTopup(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.topup, cmd = m.topup.Update(msg)
	return m, cmd
}

// updateWithdraw handles messages for the withdraw screen.
func (m *Model) updateWithdraw(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.withdraw, cmd = m.withdraw.Update(msg)
	return m, cmd
}

// transitionToDashboard switches from login to dashboard screen.
func (m *Model) transitionToDashboard() {
	m.current = screenDashboard
	m.dashboard = newDashboardScreen(m.session)
}

// View renders the current screen.
func (m *Model) View() string {
	// Enforce minimum size.
	if m.windowWidth > 0 && m.windowHeight > 0 {
		if m.windowWidth < minWidth || m.windowHeight < minHeight {
			return m.renderMinSizeWarning()
		}
	}

	// If help overlay is showing, render it on top of current screen.
	if m.showingHelp {
		return m.renderHelpOverlay()
	}

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
	case screenTopup:
		return m.topup.View()
	case screenWithdraw:
		return m.withdraw.View()
	case screenSettings:
		return m.settings.View()
	}
	return ""
}

// renderMinSizeWarning renders a minimum size warning overlay.
func (m *Model) renderMinSizeWarning() string {
	lang := m.session.Lang()

	warningStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	warningContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError)).Render(T("min_size_warning", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(T("min_size_label", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render(
			fmt.Sprintf(T("current_size_fmt", lang), m.windowWidth, m.windowHeight),
		),
	)

	return lipgloss.NewStyle().
		Width(m.windowWidth).
		Height(m.windowHeight).
		Align(lipgloss.Center, lipgloss.Center).
		Render(warningStyle.Render(warningContent))
}

// renderQuitConfirmation renders a quit confirmation dialog.
func (m *Model) renderQuitConfirmation() string {
	lang := m.session.Lang()

	// Create a centered confirmation dialog.
	dialogStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(40).
		Align(lipgloss.Center)

	dialogContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorError)).Render(T("quit_title", lang)),
		"",
		T("quit_confirm", lang),
		T("quit_cancel", lang),
	)

	return lipgloss.NewStyle().
		Width(80).
		Height(24).
		Align(lipgloss.Center, lipgloss.Center).
		Render(dialogStyle.Render(dialogContent))
}

// renderHelpOverlay renders the help keyboard shortcuts overlay.
func (m *Model) renderHelpOverlay() string {
	lang := m.session.Lang()

	helpStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(1, 3).
		Width(56).
		Align(lipgloss.Left)

	helpContent := lipgloss.JoinVertical(lipgloss.Left,
		lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorPrimary)).Width(50).Align(lipgloss.Center).Render(T("help_title", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Width(50).Render(dashLine(T("help_nav_global", lang))),
		helpItem("Esc", T("help_esc", lang)),
		helpItem("Tab ↑↓", T("help_tab", lang)),
		helpItem("Enter", T("help_enter", lang)),
		helpItem("Ctrl+C", T("help_ctrlc", lang)),
		helpItem("?", T("help_toggle_help", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Width(50).Render(dashLine(T("help_dash_shortcuts", lang))),
		helpItem("1 / T", T("help_dash_transfer", lang)),
		helpItem("2 / U", T("help_dash_topup", lang)),
		helpItem("3 / H", T("help_dash_history", lang)),
		helpItem("4 / W", T("help_dash_withdraw", lang)),
		helpItem("X", "Tutup pengingat / Dismiss nudge"),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Width(50).Render(dashLine(T("help_topup_shortcuts", lang))),
		helpItem("← →", T("help_topup_method_sel", lang)),
		helpItem("C", T("help_topup_copy_va", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Width(50).Align(lipgloss.Center).Render(T("help_close", lang)),
	)

	return lipgloss.NewStyle().
		Width(80).
		Height(24).
		Align(lipgloss.Center, lipgloss.Center).
		Render(helpStyle.Render(helpContent))
}

// helpItem returns a styled help item line.
func helpItem(key, desc string) string {
	keyStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSenpai)).
		Bold(true).
		Width(14).
		Align(lipgloss.Left)
	descStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorWhite))
	return keyStyle.Render(key) + descStyle.Render(desc)
}

// dashLine returns a dashed separator line.
func dashLine(label string) string {
	if label == "" {
		return strings.Repeat("─", 40)
	}
	padding := 40 - len(label) - 2
	if padding < 2 {
		padding = 2
	}
	left := padding / 2
	right := padding - left
	return strings.Repeat("─", left) + " " + label + " " + strings.Repeat("─", right)
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
