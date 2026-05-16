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

// Balance and nudge refresh interval (30 seconds).
const balanceRefreshInterval = 30 * time.Second

// Nudge-related focus indices.
const (
	// focusQuickActionsStart is the first quick action index.
	// nudgeActionFocus is the index assigned for the nudge action hint focusable element.
	nudgeActionFocus   = 10
	nudgeDismissFocus  = 11
	nudgeFocusElements = 2 // action hint + dismiss hint
)

// severity colors for nudge display.
const (
	nudgeColorInfo     = colorPrimary   // blue
	nudgeColorWarning  = colorSenpai    // yellow/gold
	nudgeColorCritical = colorError     // red
)

// severityIcons maps nudge severity to display icons.
var severityIcons = map[string]string{
	"info":     "ℹ",
	"warning":  "⚠",
	"critical": "🔴",
}

// severityColors maps nudge severity to display colors.
var severityColors = map[string]string{
	"info":     nudgeColorInfo,
	"warning":  nudgeColorWarning,
	"critical": nudgeColorCritical,
}

// nudgeActionTriggeredMsg is sent when user activates the nudge action hint.
type nudgeActionTriggeredMsg struct{}

// nudgeDismissedMsg is sent when user dismisses a nudge.
type nudgeDismissedMsg struct {
	message string
}

// dashboardScreen is the dashboard screen model.
type dashboardScreen struct {
	session    *Session
	focusIndex int // which quick action is focused (-1 = none/nav area)
	activeTab  int // current navigation tab
	errMsg     string
	tip        string

	// Nudge state.
	nudges    []NudgeItem // current nudges from API
	nudgeErr  string      // nudge fetch error
	nudgeIdx  int         // which nudge is being displayed (for future multi-nudge pagination)
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

// --- Nudge commands ---

// nudgeUpdatedMsg carries fetched nudge results.
type nudgeUpdatedMsg struct {
	nudges []NudgeItem
	err    string
}

// fetchNudgeCmd fetches nudges from the API.
func fetchNudgeCmd(token string) tea.Cmd {
	return func() tea.Msg {
		nudges, err := GetNudges(token)
		if err != nil {
			return nudgeUpdatedMsg{err: err.Error()}
		}
		return nudgeUpdatedMsg{nudges: nudges}
	}
}

// displayedNudges returns nudges that have not been dismissed this session.
func (d *dashboardScreen) displayedNudges() []NudgeItem {
	if len(d.nudges) == 0 {
		return nil
	}
	var visible []NudgeItem
	for _, n := range d.nudges {
		if !d.session.DismissedNudges[n.Message] {
			visible = append(visible, n)
		}
	}
	return visible
}

func (d *dashboardScreen) Init() tea.Cmd {
	// Fetch balance and nudges immediately on init.
	return tea.Batch(
		fetchBalanceCmd(d.session.Token),
		fetchNudgeCmd(d.session.Token),
		balanceTick(),
	)
}

func (d *dashboardScreen) Update(msg tea.Msg) (*dashboardScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return d, tea.Quit
		}

		// 'x' key dismisses the current visible nudge regardless of focus.
		if msg.String() == "x" {
			visible := d.displayedNudges()
			if len(visible) > 0 && d.nudgeIdx < len(visible) {
				n := visible[d.nudgeIdx]
				if n.Dismissible {
					d.session.DismissedNudges[n.Message] = true
					// Reset focus if we were on nudge elements.
					if d.focusIndex >= nudgeActionFocus {
						d.focusIndex = -1
					}
				}
			}
			return d, nil
		}

		switch msg.String() {
		case "tab", "down":
			totalFocusElements := len(quickActionIcons)

			// Check if there are visible nudges with action hint.
			visible := d.displayedNudges()
			hasNudgeElements := false
			if len(visible) > 0 {
				hasNudgeElements = len(visible) > 0
			}

			d.focusIndex++

			if hasNudgeElements {
				// Focus cycle: quick actions → nudge action hint → nudge dismiss → wrap to -1
				switch {
				case d.focusIndex < totalFocusElements:
					// Quick action range.
				case d.focusIndex == totalFocusElements:
					d.focusIndex = nudgeActionFocus
				case d.focusIndex == nudgeActionFocus+1:
					d.focusIndex = nudgeDismissFocus
				default:
					d.focusIndex = -1
				}
			} else {
				// No nudge, just cycle quick actions.
				if d.focusIndex >= totalFocusElements {
					d.focusIndex = -1
				}
			}
			return d, nil

		case "shift+tab", "up":
			totalFocusElements := len(quickActionIcons)
			visible := d.displayedNudges()
			hasNudgeElements := len(visible) > 0

			d.focusIndex--

			if hasNudgeElements {
				switch {
				case d.focusIndex >= 0 && d.focusIndex < totalFocusElements:
					// Quick action range.
				case d.focusIndex == totalFocusElements-1:
					// Coming from nudge dismiss or after wrap.
					if d.focusIndex < 0 {
						d.focusIndex = nudgeDismissFocus
					}
				case d.focusIndex == nudgeActionFocus-1:
					d.focusIndex = nudgeDismissFocus
				case d.focusIndex < 0:
					d.focusIndex = nudgeDismissFocus
				default:
					// Keep in nudge range.
				}
			} else {
				if d.focusIndex < 0 {
					d.focusIndex = totalFocusElements - 1
				}
			}

			// Clamp for nudge range.
			if hasNudgeElements && d.focusIndex < 0 {
				d.focusIndex = nudgeDismissFocus
			}
			return d, nil

		case "enter", " ":
			if d.focusIndex >= 0 && d.focusIndex < len(quickActionIcons) {
				actionIDs := []int{ActionTransfer, ActionTopUp, ActionWithdraw, ActionHistory}
				return d, func() tea.Msg {
					return quickActionSelectedMsg{actionID: actionIDs[d.focusIndex]}
				}
			}

			// Nudge action hint activation.
			if d.focusIndex == nudgeActionFocus {
				visible := d.displayedNudges()
				if len(visible) > 0 && d.nudgeIdx < len(visible) {
					n := visible[d.nudgeIdx]
					if n.Action != "" {
						// Trigger action - navigate based on action text.
						// For now, we just send a message that can be handled.
						return d, func() tea.Msg {
							return nudgeActionTriggeredMsg{}
						}
					}
				}
			}

			// Nudge dismiss focus activation.
			if d.focusIndex == nudgeDismissFocus {
				visible := d.displayedNudges()
				if len(visible) > 0 && d.nudgeIdx < len(visible) {
					n := visible[d.nudgeIdx]
					if n.Dismissible {
						d.session.DismissedNudges[n.Message] = true
						d.focusIndex = -1
					}
				}
			}
			return d, nil

		case "esc":
			return d, nil

		case "1", "t":
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionTransfer}
			}
		case "2", "u":
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionTopUp}
			}
		case "3", "h":
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionHistory}
			}
		case "4", "w":
			return d, func() tea.Msg {
				return quickActionSelectedMsg{actionID: ActionWithdraw}
			}
		case "5", "s":
			return d, func() tea.Msg {
				return navigateToSettingsMsg{}
			}
		}

	case balanceTickMsg:
		// Periodic balance refresh — also fetch nudges.
		return d, tea.Batch(
			fetchBalanceCmd(d.session.Token),
			fetchNudgeCmd(d.session.Token),
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

	case nudgeUpdatedMsg:
		if msg.err != "" {
			d.nudgeErr = d.session.T("error_nudge_fetch")
		} else {
			d.nudges = msg.nudges
			d.nudgeErr = ""
			// Reset nudge index if it's out of bounds.
			visible := d.displayedNudges()
			if d.nudgeIdx >= len(visible) {
				d.nudgeIdx = 0
			}
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

	// Nudge card.
	visible := d.displayedNudges()
	if len(visible) > 0 {
		b.WriteString(d.renderNudgeCard(visible[d.nudgeIdx], lang))
		b.WriteString("\n")
	}

	// Nudge fetch error (less prominent than nudge card).
	if d.nudgeErr != "" && d.errMsg == "" {
		b.WriteString(lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Width(60).Align(lipgloss.Center).Render(d.nudgeErr))
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

// renderNudgeCard renders a single nudge as a styled card.
func (d *dashboardScreen) renderNudgeCard(n NudgeItem, lang string) string {
	severityColor, ok := severityColors[n.Severity]
	if !ok {
		severityColor = colorSecondary
	}

	icon, ok := severityIcons[n.Severity]
	if !ok {
		icon = "ℹ"
	}

	// Build the nudge card content.
	// Line 1: icon + message.
	iconStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(severityColor)).Bold(true)

	cardContent := lipgloss.JoinVertical(lipgloss.Left,
		iconStyle.Render(icon+" "+n.Message),
		"",  // spacing
		d.renderNudgeFooter(n, lang, severityColor),
	)

	card := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(severityColor)).
		Padding(0, 1).
		Width(60).
		MarginTop(0).
		MarginBottom(0)

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(card.Render(cardContent))
}

// renderNudgeFooter renders the action hint and dismiss hint for a nudge.
func (d *dashboardScreen) renderNudgeFooter(n NudgeItem, lang string, severityColor string) string {
	var footerParts []string

	// Action hint - styled as clickable if action is non-empty.
	if n.Action != "" {
		actionHint := fmt.Sprintf(d.session.T("nudge_action_hint"), n.Action)
		var actionStyle lipgloss.Style
		if d.focusIndex == nudgeActionFocus {
			actionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(colorWhite)).
				Background(lipgloss.Color(severityColor)).
				Padding(0, 1)
		} else {
			actionStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color(severityColor)).
				Padding(0, 1)
		}
		footerParts = append(footerParts, actionStyle.Render(actionHint))
	}

	// Dismiss hint.
	dismissText := d.session.T("nudge_dismiss")
	if n.Dismissible {
		var dismissStyle lipgloss.Style
		if d.focusIndex == nudgeDismissFocus {
			dismissStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorWhite)).
				Background(lipgloss.Color(colorMuted)).
				Padding(0, 1)
		} else {
			dismissStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorMuted)).
				Padding(0, 1)
		}
		footerParts = append(footerParts, dismissStyle.Render(dismissText))
	}

	if len(footerParts) == 0 {
		return ""
	}

	return lipgloss.JoinHorizontal(lipgloss.Center, footerParts...)
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
