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

// quickAction represents a quick action button on the dashboard.
type quickAction struct {
	id    int
	label string
	icon  string
}

var quickActions = []quickAction{
	{id: ActionTransfer, label: "Transfer", icon: "⇄"},
	{id: ActionTopUp, label: "Top Up", icon: "↓"},
	{id: ActionWithdraw, label: "Withdraw", icon: "↑"},
	{id: ActionHistory, label: "History", icon: "☰"},
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

// navTab represents a navigation tab.
type navTab struct {
	id    int
	label string
}

var navTabs = []navTab{
	{id: NavLogin, label: "Profile"},
	{id: NavDashboard, label: "Dashboard"},
	{id: NavTransfer, label: "Transfer"},
	{id: NavHistory, label: "History"},
	{id: NavTopUp, label: "Top Up"},
	{id: NavWithdraw, label: "Withdraw"},
}

// Senpai tips in Indonesian.
var senpaiTips = []string{
	"Hemat pangkal kaya!",
	"Catat pengeluaranmu hari ini",
	"Menabung 10% penghasilan lebih baik daripada tidak sama sekali",
	"Gunakan uang digital untuk mengurangi pengeluaran tak terduga",
	"Selalu periksa saldo sebelum bertransaksi",
	"Bijak dalam bertransaksi adalah kunci keuangan sehat",
	"Jangan lupa isi saldo untuk kebutuhan darurat",
	"Pantau histori transaksi untuk evaluasi keuangan",
}

// Balance refresh interval (30 seconds).
const balanceRefreshInterval = 30 * time.Second

// dashboardScreen is the dashboard screen model.
type dashboardScreen struct {
	session          *Session
	focusIndex       int // which quick action is focused (-1 = none/nav area)
	activeTab        int // current navigation tab
	errMsg           string
	tip              string
	loading          bool
	lastBalanceFetch time.Time
}

// newDashboardScreen creates a new dashboard screen.
func newDashboardScreen(session *Session) *dashboardScreen {
	d := &dashboardScreen{
		session:    session,
		focusIndex: -1,
		activeTab:  NavDashboard,
		tip:        randomTip(),
	}

	// Trigger initial balance fetch.
	return d
}

// randomTip returns a random senpai tip.
func randomTip() string {
	return senpaiTips[rand.Intn(len(senpaiTips))]
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
			if d.focusIndex < len(quickActions)-1 {
				d.focusIndex++
			} else {
				d.focusIndex = 0
			}
			return d, nil

		case "shift+tab", "up":
			if d.focusIndex > 0 {
				d.focusIndex--
			} else {
				d.focusIndex = len(quickActions) - 1
			}
			return d, nil

		case "enter", " ":
			if d.focusIndex >= 0 && d.focusIndex < len(quickActions) {
				action := quickActions[d.focusIndex]
				return d, func() tea.Msg {
					return quickActionSelectedMsg{actionID: action.id}
				}
			}
			return d, nil

		case "esc":
			// Esc could go back to login/logout confirmation.
			// For now, do nothing special on dashboard.
			return d, nil

		case "1":
			d.activeTab = NavDashboard
			return d, nil
		case "2":
			d.activeTab = NavTransfer
			return d, nil
		case "3":
			d.activeTab = NavHistory
			return d, nil
		case "4":
			d.activeTab = NavTopUp
			return d, nil
		case "5":
			d.activeTab = NavWithdraw
			return d, nil
		}

	case balanceTickMsg:
		// Periodic balance refresh.
		return d, tea.Batch(
			fetchBalanceCmd(d.session.Token),
			balanceTick(),
		)

	case balanceUpdatedMsg:
		if msg.err != "" {
			d.errMsg = "Gagal memperbarui saldo"
		} else {
			d.session.SetBalance(msg.balanceSen, msg.version)
			d.errMsg = ""
		}
		return d, nil

	case quickActionSelectedMsg:
		// For now, non-implemented actions show a message.
		// Later features will implement these screens.
		d.errMsg = fmt.Sprintf("Fitur %s belum tersedia", quickActions[msg.actionID].label)
		return d, nil
	}

	return d, nil
}

func (d *dashboardScreen) View() string {
	var b strings.Builder

	// Navigation tabs at top.
	b.WriteString(d.renderNav())
	b.WriteString("\n")

	// Greeting.
	greeting := fmt.Sprintf("Halo, %s!", maskPhone(d.session.Phone))
	b.WriteString(GreetingStyle.Render(greeting))
	b.WriteString("\n")

	// Balance.
	b.WriteString(BalanceLabelStyle.Render("Saldo Anda"))
	b.WriteString("\n")
	b.WriteString(BalanceStyle.Render(formatIDR(d.session.BalanceSen)))
	b.WriteString("\n")

	// Error message area.
	if d.errMsg != "" {
		b.WriteString(ErrorStyle.Render(d.errMsg))
		b.WriteString("\n")
	}

	// Quick actions.
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render("\n" + d.renderQuickActions()))
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
				lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorSenpai)).Render("💡 Senpai Tip"),
				"",
				d.tip,
			),
		),
	))
	b.WriteString("\n")

	// Help text.
	b.WriteString(HelpStyle.Render("Tab: navigasi • Enter: pilih • 1-5: tab • Ctrl+C: keluar"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

// renderNav renders the navigation tabs.
func (d *dashboardScreen) renderNav() string {
	var tabs []string
	for _, tab := range navTabs {
		var style lipgloss.Style
		if tab.id == d.activeTab {
			style = NavTabActiveStyle
		} else if tab.id == NavLogin || tab.id == NavDashboard {
			// Login (Profile) and Dashboard are available.
			style = NavTabStyle.Foreground(lipgloss.Color(colorSecondary))
		} else {
			// Other tabs are disabled (not yet implemented).
			style = NavTabDisabledStyle
		}
		tabs = append(tabs, style.Render(tab.label))
	}
	return lipgloss.JoinHorizontal(lipgloss.Center, tabs...)
}

// renderQuickActions renders the quick action buttons.
func (d *dashboardScreen) renderQuickActions() string {
	var actions []string
	for i, action := range quickActions {
		var style lipgloss.Style
		if d.focusIndex == i {
			style = FocusedQuickActionStyle
		} else {
			style = QuickActionStyle
		}
		actions = append(actions, style.Render(action.icon+" "+action.label))
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
