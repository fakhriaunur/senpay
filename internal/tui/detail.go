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

// txTypeDisplay returns the Indonesian display name for a transaction type.
func txTypeDisplay(txType string) string {
	switch types.TxType(txType) {
	case types.TxTypeTransfer:
		return "Transfer"
	case types.TxTypeTopup:
		return "Top Up"
	case types.TxTypeWithdraw:
		return "Withdraw"
	case types.TxTypeFee:
		return "Biaya"
	default:
		return txType
	}
}

// statusDisplay returns the Indonesian display name for a status.
func statusDisplay(status string) string {
	switch types.TxStatus(status) {
	case types.TxStatusCommitted:
		return "Berhasil"
	case types.TxStatusPending:
		return "Pending"
	case types.TxStatusFailed:
		return "Gagal"
	case types.TxStatusCompensated:
		return "Dikembalikan"
	case types.TxStatusTimeout:
		return "Timeout"
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

	b.WriteString(TitleStyle.Render("Detail Transaksi"))
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
		labelValue("ID Transaksi", tx.ID),
		"",
		labelValue("Tipe", txTypeDisplay(tx.TxType)),
		"",
		labelValue("Jumlah", amountPrefix+formatAmountSen(tx.AmountSen), lipgloss.Color(amountColor)),
		"",
	)
	if tx.CounterpartyPhone != "" {
		rows = append(rows, labelValue("Counterparty", tx.CounterpartyPhone), "")
	}
	rows = append(rows,
		labelValue("Status", statusDisplay(tx.Status), lipgloss.Color(statusColor(tx.Status))),
		"",
		labelValue("Waktu", formatTime(tx.CreatedAt)),
		"",
	)
	if tx.CommittedAt != "" {
		rows = append(rows, labelValue("Diproses", formatTime(tx.CommittedAt)), "")
	}

	detail := lipgloss.JoinVertical(lipgloss.Left, rows...)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		detailStyle.Render(detail),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render("Esc: kembali ke daftar"))

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
