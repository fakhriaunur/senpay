package tui

import (
	"fmt"
	"math/rand"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Quick action IDs.
const (
	ActionTransfer = iota
	ActionTopUp
	ActionWithdraw
	ActionHistory
)

// quickActionI18nKeys maps action IDs to their i18n keys.
var quickActionI18nKeys = map[int]string{
	ActionTransfer: "quick_action_transfer",
	ActionTopUp:    "quick_action_topup",
	ActionWithdraw: "quick_action_withdraw",
	ActionHistory:  "quick_action_history",
}

// quickActionIcons maps action IDs to their display icons.
var quickActionIcons = map[int]string{
	ActionTransfer: "⇄",
	ActionTopUp:    "↓",
	ActionWithdraw: "↑",
	ActionHistory:  "☰",
}

// Navigation tab IDs.
const (
	NavLogin = iota
	NavDashboard
	NavTransfer
	NavHistory
	NavTopUp
	NavWithdraw
)

// navTabI18nKeys maps tab IDs to their i18n keys.
var navTabI18nKeys = map[int]string{
	NavLogin:     "nav_profile",
	NavDashboard: "nav_dashboard",
	NavTransfer:  "nav_transfer",
	NavHistory:   "nav_history",
	NavTopUp:     "nav_topup",
	NavWithdraw:  "nav_withdraw",
}

// Senpai tips i18n keys.
var senpaiTipKeys = []string{
	"senpai_tip_1",
	"senpai_tip_2",
	"senpai_tip_3",
	"senpai_tip_4",
	"senpai_tip_5",
	"senpai_tip_6",
	"senpai_tip_7",
	"senpai_tip_8",
}

// Balance refresh interval (30 seconds).
const balanceRefreshInterval = 30 * time.Second

// dashboardScreen is the dashboard screen model.
type dashboardScreen struct {
	session    *Session
	focusIndex int // which quick action is focused (-1 = none/nav area)
	activeTab  int // current navigation tab
	errMsg     string
	tip        string
}

// newDashboardScreen creates a new dashboard screen.
func newDashboardScreen(session *Session) *dashboardScreen {
	d := &dashboardScreen{
		session:    session,
		focusIndex: -1,
		activeTab:  NavDashboard,
		tip:        randomTip(session.Lang()),
	}

	// Trigger initial balance fetch.
	return d
}

// randomTip returns a random senpai tip in the given language.
func randomTip(lang string) string {
	return T(senpaiTipKeys[rand.Intn(len(senpaiTipKeys))], lang)
}

// balanceUpdatedMsg carries updated balance info.
type balanceUpdatedMsg struct {
	balanceSen int64
	version    int
	err        string
}

// balanceTick is sent periodically to trigger balance refresh.
type balanceTickMsg struct{}

// fetchBalanceCmd fetches the balance from the API.
func fetchBalanceCmd(token string) tea.Cmd {
	return func() tea.Msg {
		balance, version, err := GetBalance(token)
		if err != nil {
			return balanceUpdatedMsg{err: err.Error()}
		}
		return balanceUpdatedMsg{
			balanceSen: balance,
			version:    version,
		}
	}
}

// balanceTick creates a tick for periodic balance refresh.
func balanceTick() tea.Cmd {
	return tea.Tick(balanceRefreshInterval, func(t time.Time) tea.Msg {
		return balanceTickMsg{}
	})
}

// formatIDR formats a sen amount as Indonesian Rupiah.
// Example: 100000 sen → "Rp 1.000.000"
func formatIDR(sen int64) string {
	idr := sen / 100 // Convert sen to IDR
	if idr < 0 {
		idr = 0
	}
	s := fmt.Sprintf("%d", idr)

	// Add thousand separators (.) per Indonesian format.
	var result strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			result.WriteRune('.')
		}
		result.WriteRune(c)
	}

	return "Rp " + result.String()
}

// quickActionSelectedMsg is sent when a quick action is selected.
type quickActionSelectedMsg struct {
	actionID int
}

func (d *dashboardScreen) Init() tea.Cmd {
	// Fetch balance immediately on init.
	return tea.Batch(
		fetchBalanceCmd(d.session.Token),
		balanceTick(),
	)
}

func (d *dashboardScreen) Update(msg tea.Msg) (*dashboardScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return d, tea.Quit
		}

		switch msg.String() {
		case "tab", "down":
			// Cycle through quick actions.
			if d.focusIndex < len(quickActionIcons)-1 {
				d.focusIndex++
			} else {
				d.focusIndex = 0
			}
			return d, nil

		case "shift+tab", "up":
			if d.focusIndex > 0 {
				d.focusIndex--
			} else {
				d.focusIndex = len(quickActionIcons) - 1
			}
			return d, nil

		case "enter", " ":
			if d.focusIndex >= 0 && d.focusIndex < len(quickActionIcons) {
				actionIDs := []int{ActionTransfer, ActionTopUp, ActionWithdraw, ActionHistory}
				return d, func() tea.Msg {
					return quickActionSelectedMsg{actionID: actionIDs[d.focusIndex]}
				}
			}
			return d, nil

		case "esc":
			// Esc could go back to login/logout confirmation.
			// For now, do nothing special on dashboard.
			return d, nil

		case "1", "t":
			// Navigate to Transfer.
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionTransfer}
			}
		case "2", "u":
			// Navigate to Top Up.
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionTopUp}
			}
		case "3", "h":
			// Navigate to History.
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionHistory}
			}
		case "4", "w":
			// Navigate to Withdraw.
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionWithdraw}
			}
		case "5", "s":
			// Navigate to Settings.
			return d, func() tea.Msg {
				return navigateToSettingsMsg{}
			}
		}

	case balanceTickMsg:
		// Periodic balance refresh.
		return d, tea.Batch(
			fetchBalanceCmd(d.session.Token),
			balanceTick(),
		)

	case balanceUpdatedMsg:
		if msg.err != "" {
			d.errMsg = d.session.T("error_balance_fetch")
		} else {
			d.session.SetBalance(msg.balanceSen, msg.version)
			d.errMsg = ""
		}
		return d, nil

	}

	return d, nil
}

func (d *dashboardScreen) View() string {
	var b strings.Builder
	lang := d.session.Lang()

	// Navigation tabs at top.
	b.WriteString(d.renderNav(lang))
	b.WriteString("\n")

	// Greeting.
	greeting := d.session.T("greeting", maskPhone(d.session.Phone))
	b.WriteString(GreetingStyle.Render(greeting))
	b.WriteString("\n")

	// Balance.
	b.WriteString(BalanceLabelStyle.Render(d.session.T("balance_label")))
	b.WriteString("\n")
	b.WriteString(BalanceStyle.Render(formatIDR(d.session.BalanceSen)))
	b.WriteString("\n")

	// Error message area.
	if d.errMsg != "" {
		b.WriteString(ErrorStyle.Render(d.errMsg))
		b.WriteString("\n")
	}

	// Quick actions.
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render("\n" + d.renderQuickActions(lang)))
	b.WriteString("\n")

	// Senpai tip.
	tipBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSenpai)).
		Padding(0, 2).
		Width(56).
		Align(lipgloss.Center).
		MarginTop(1)
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		tipBox.Render(
			lipgloss.JoinVertical(lipgloss.Center,
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorSenpai)).Render(d.session.T("senpai_tip_title")),
				"",
				d.tip,
			),
		),
	))
	b.WriteString("\n")

	// Help text.
	b.WriteString(HelpStyle.Render(d.session.T("help_dashboard")))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

// renderNav renders the navigation tabs.
func (d *dashboardScreen) renderNav(lang string) string {
	var tabs []string
	tabIDs := []int{NavLogin, NavDashboard, NavTransfer, NavHistory, NavTopUp, NavWithdraw}
	for _, id := range tabIDs {
		label := T(navTabI18nKeys[id], lang)
		var style lipgloss.Style
		if id == d.activeTab {
			style = NavTabActiveStyle
		} else {
			style = NavTabStyle.Foreground(lipgloss.Color(colorSecondary))
		}
		tabs = append(tabs, style.Render(label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

// renderQuickActions renders the quick action buttons.
func (d *dashboardScreen) renderQuickActions(lang string) string {
	var actions []string
	actionIDs := []int{ActionTransfer, ActionTopUp, ActionWithdraw, ActionHistory}
	for i, id := range actionIDs {
		label := T(quickActionI18nKeys[id], lang)
		icon := quickActionIcons[id]
		var style lipgloss.Style
		if d.focusIndex == i {
			style = FocusedQuickActionStyle
		} else {
			style = QuickActionStyle
		}
		actions = append(actions, style.Render(icon+" "+label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, actions...)
}

// maskPhone masks the middle digits of a phone number for display.
func maskPhone(phone string) string {
	if len(phone) <= 6 {
		return phone
	}
	return phone[:4] + "****" + phone[len(phone)-3:]
}
