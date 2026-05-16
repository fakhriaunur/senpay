// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"strings"

	"senpay/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// detailScreen shows full transaction detail.
type detailScreen struct {
	session *Session
	tx      TransactionItem
}

// newDetailScreen creates a new transaction detail screen.
func newDetailScreen(session *Session, tx TransactionItem) *detailScreen {
	return &detailScreen{
		session: session,
		tx:      tx,
	}
}

func (d *detailScreen) Init() tea.Cmd {
	return nil
}

func (d *detailScreen) Update(msg tea.Msg) (*detailScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return d, tea.Quit
		}

		if msg.String() == "esc" {
			return d, func() tea.Msg {
				return navigateToHistoryMsg{}
			}
		}
	}

	return d, nil
}

// navigateToHistoryMsg signals parent to navigate back to history list.
type navigateToHistoryMsg struct{}

// txTypeDisplay returns the display name for a transaction type in the given language.
func txTypeDisplay(txType, lang string) string {
	switch types.TxType(txType) {
	case types.TxTypeTransfer:
		return T("type_transfer", lang)
	case types.TxTypeTopup:
		return T("type_topup", lang)
	case types.TxTypeWithdraw:
		return T("type_withdraw", lang)
	case types.TxTypeFee:
		return T("type_fee", lang)
	default:
		return txType
	}
}

// statusDisplay returns the display name for a status in the given language.
func statusDisplay(status, lang string) string {
	switch types.TxStatus(status) {
	case types.TxStatusCommitted:
		return T("status_committed", lang)
	case types.TxStatusPending:
		return T("status_pending", lang)
	case types.TxStatusFailed:
		return T("status_failed", lang)
	case types.TxStatusCompensated:
		return T("status_compensated", lang)
	case types.TxStatusTimeout:
		return T("status_timeout", lang)
	default:
		return status
	}
}

// statusColor returns the lipgloss color for a status.
func statusColor(status string) string {
	switch types.TxStatus(status) {
	case types.TxStatusCommitted:
		return colorSuccess
	case types.TxStatusPending:
		return colorSenpai
	case types.TxStatusFailed, types.TxStatusCompensated:
		return colorError
	default:
		return colorSecondary
	}
}

// isCredit returns true if the tx is incoming (credit) for the current user.
// Simplified: topup is credit, transfer/withdraw/fee are debit.
func isCredit(txType string) bool {
	switch types.TxType(txType) {
	case types.TxTypeTopup:
		return true
	default:
		return false
	}
}

func (d *detailScreen) View() string {
	var b strings.Builder
	lang := d.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_detail", lang)))
	b.WriteString("\n\n")

	tx := d.tx
	detailStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(1, 3).
		Width(58).
		Align(lipgloss.Left)

	// Build detail rows.
	var rows []string

	// Amount with color.
	amountColor := colorError
	amountPrefix := "- "
	if isCredit(tx.TxType) {
		amountColor = colorSuccess
		amountPrefix = "+ "
	}

	rows = append(rows,
		labelValue(T("label_tx_id", lang), tx.ID),
		"",
		labelValue(T("label_type", lang), txTypeDisplay(tx.TxType, lang)),
		"",
		labelValue(T("label_amount", lang), amountPrefix+formatAmountSen(tx.AmountSen), lipgloss.Color(amountColor)),
		"",
	)
	if tx.CounterpartyPhone != "" {
		rows = append(rows, labelValue(T("label_counterparty", lang), tx.CounterpartyPhone), "")
	}
	rows = append(rows,
		labelValue(T("label_status", lang), statusDisplay(tx.Status, lang), lipgloss.Color(statusColor(tx.Status))),
		"",
		labelValue(T("label_time", lang), formatTime(tx.CreatedAt)),
		"",
	)
	if tx.CommittedAt != "" {
		rows = append(rows, labelValue(T("label_processed", lang), formatTime(tx.CommittedAt)), "")
	}

	detail := lipgloss.JoinVertical(lipgloss.Left, rows...)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		detailStyle.Render(detail),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render(T("help_detail", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

// labelValue returns a styled label: value pair.
func labelValue(label, value string, valueColor ...lipgloss.Color) string {
	labelStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorSecondary)).
		Width(16)

	valStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(colorWhite)).
		Bold(true)

	if len(valueColor) > 0 {
		valStyle = valStyle.Foreground(valueColor[0])
	}

	return labelStyle.Render(label) + ": " + valStyle.Render(value)
}

// formatTime formats an ISO timestamp to a friendlier display.
func formatTime(iso string) string {
	if len(iso) >= 19 {
		// "2024-01-15T10:30:00Z" -> "15/01/2024 10:30:00"
		date := iso[:10] // YYYY-MM-DD
		time := iso[11:19] // HH:MM:SS
		// Convert YYYY-MM-DD to DD/MM/YYYY
		parts := strings.Split(date, "-")
		if len(parts) == 3 {
			return parts[2] + "/" + parts[1] + "/" + parts[0] + " " + time
		}
		return date + " " + time
	}
	return iso
}
