// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"strings"

	"senpay/internal/types"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// historyScreen is the transaction history list screen.
type historyScreen struct {
	session      *Session
	items        []TransactionItem
	currentCursor string // cursor used to load current page ("" for first page)
	nextCursor   string // cursor for the next page (from API response)
	hasMore      bool
	prevCursors  []string // stack of cursors for backward navigation
	errMsg       string
	loading      bool
	selected     int // index of selected item in items list
	loaded       bool
}

// newHistoryScreen creates a new history screen.
func newHistoryScreen(session *Session) *historyScreen {
	return &historyScreen{
		session: session,
		items:   make([]TransactionItem, 0),
	}
}

// historyLoadedMsg is sent when transaction list loads.
type historyLoadedMsg struct {
	items      []TransactionItem
	cursor     string // cursor that was SENT to the API ("" for first page)
	nextCursor string
	hasMore    bool
}

// historyErrMsg is sent when transaction list fails to load.
type historyErrMsg struct {
	err string
}

// navigateToDashboardMsg signals parent to navigate to dashboard.
type navigateToDashboardMsg struct{}

// viewDetailMsg signals parent to navigate to transaction detail.
type viewDetailMsg struct {
	tx TransactionItem
}

// loadPageCmd fetches a page of transactions.
func loadPageCmd(token, cursor, lang string) tea.Cmd {
	return func() tea.Msg {
		result, err := GetTransactions(token, cursor, types.PageDefaultLimit)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") {
				return historyErrMsg{err: T("error_network", lang)}
			}
			return historyErrMsg{err: T("error_load_history", lang)}
		}
		return historyLoadedMsg{
			items:      result.Data,
			cursor:     cursor,
			nextCursor: result.NextCursor,
			hasMore:    result.HasMore,
		}
	}
}

func (h *historyScreen) Init() tea.Cmd {
	if !h.loaded {
		h.loading = true
		return loadPageCmd(h.session.Token, "", h.session.Lang())
	}
	return nil
}

func (h *historyScreen) Update(msg tea.Msg) (*historyScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return h, tea.Quit
		}

		// Don't process keyboard while loading.
		if h.loading {
			return h, nil
		}

		switch msg.String() {
		case "down", "j":
			if h.selected < len(h.items)-1 {
				h.selected++
			} else if h.hasMore && !h.loading {
				// Save current page's cursor, then load next page.
				h.loading = true
				h.prevCursors = append(h.prevCursors, h.currentCursor)
				return h, loadPageCmd(h.session.Token, h.nextCursor, h.session.Lang())
			}
			return h, nil

		case "up", "k":
			if h.selected > 0 {
				h.selected--
			} else if len(h.prevCursors) > 0 && !h.loading {
				// Go to previous page: pop cursor from stack and reload.
				h.loading = true
				prevCursor := h.prevCursors[len(h.prevCursors)-1]
				h.prevCursors = h.prevCursors[:len(h.prevCursors)-1]
				return h, loadPageCmd(h.session.Token, prevCursor, h.session.Lang())
			}
			return h, nil

		case "enter":
			if len(h.items) > 0 && h.selected >= 0 && h.selected < len(h.items) {
				tx := h.items[h.selected]
				return h, func() tea.Msg {
					return viewDetailMsg{tx: tx}
				}
			}
			return h, nil

		case "esc":
			return h, func() tea.Msg {
				return navigateToDashboardMsg{}
			}
		}

	case historyLoadedMsg:
		h.loading = false
		h.loaded = true
		h.items = msg.items
		h.currentCursor = msg.cursor // cursor used to load this page
		h.nextCursor = msg.nextCursor // cursor for loading the next page
		h.hasMore = msg.hasMore
		h.selected = 0
		h.errMsg = ""
		return h, nil

	case historyErrMsg:
		h.loading = false
		h.errMsg = msg.err
		return h, nil
	}

	return h, nil
}

// isDebit returns true if the transaction is a debit (money going out) for the current user.
// The sender is the current user if sender_id matches the session user's... actually we
// don't store user ID in session. We determine by tx_type: transfer and fee with amount_sen
// positive means debit (money out) when we're the sender.
// For the TUI display, we simplify: transfers and fee entries are debits (outgoing),
// topup entries are credits (incoming).
func (h *historyScreen) isDebit(tx TransactionItem) bool {
	switch types.TxType(tx.TxType) {
	case types.TxTypeTransfer, types.TxTypeFee, types.TxTypeWithdraw:
		return true
	case types.TxTypeTopup:
		return false
	default:
		return false
	}
}

// counterpartyName returns the counterparty display name for a transaction.
func (h *historyScreen) counterpartyName(tx TransactionItem) string {
	if tx.CounterpartyPhone != "" {
		return tx.CounterpartyPhone
	}
	switch types.TxType(tx.TxType) {
	case types.TxTypeTopup:
		return h.session.T("counterparty_topup")
	case types.TxTypeWithdraw:
		return h.session.T("counterparty_withdraw")
	case types.TxTypeFee:
		return h.session.T("counterparty_fee")
	default:
		return "-"
	}
}

// statusIcon returns a visual status icon.
func (h *historyScreen) statusIcon(status string) string {
	switch types.TxStatus(status) {
	case types.TxStatusCommitted:
		return "✓"
	case types.TxStatusPending:
		return "⏳"
	case types.TxStatusFailed:
		return "✕"
	case types.TxStatusCompensated:
		return "↩"
	default:
		return "?"
	}
}

func (h *historyScreen) View() string {
	var b strings.Builder
	lang := h.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_history", lang)))
	b.WriteString("\n\n")

	if h.errMsg != "" {
		b.WriteString(ErrorStyle.Render(h.errMsg))
		b.WriteString("\n\n")
		return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
	}

	if h.loading {
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).
			Render(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorPrimary)).
				Render(T("loading", lang)),
			))
		return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
	}

	if len(h.items) == 0 {
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).
			Render(lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorSecondary)).
				Render(T("empty_history", lang)),
			))
		b.WriteString("\n\n")
		b.WriteString(HelpStyle.Render(T("help_detail", lang)))
		return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
	}

	// Render table header.
	headerStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorSecondary)).
		Padding(0, 1)

	header := lipgloss.JoinHorizontal(lipgloss.Left,
		headerStyle.Width(14).Render(T("col_date", lang)),
		headerStyle.Width(22).Render(T("col_counterparty", lang)),
		headerStyle.Width(16).Align(lipgloss.Right).Render(T("col_amount", lang)),
		headerStyle.Width(8).Align(lipgloss.Center).Render(T("col_status", lang)),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(header))
	b.WriteString("\n")

	// Render rows.
	for i, tx := range h.items {
		rowStyle := lipgloss.NewStyle().Padding(0, 1)
		if i == h.selected {
			rowStyle = rowStyle.Background(lipgloss.Color(colorHighlight))
		}

		// Format date (take first 10 chars of timestamp).
		date := tx.CreatedAt
		if len(date) > 10 {
			// Try to parse and re-format nicely.
			date = tx.CreatedAt[:10] // YYYY-MM-DD
		}

		counterparty := h.counterpartyName(tx)
		if len(counterparty) > 20 {
			counterparty = counterparty[:20]
		}

		// Amount with sign and color.
		var amountStr string
		var amountColor string
		if h.isDebit(tx) {
			amountStr = "-" + formatAmountSen(tx.AmountSen)
			amountColor = colorError // red for debit
		} else {
			amountStr = "+" + formatAmountSen(tx.AmountSen)
			amountColor = colorSuccess // green for credit
		}

		statusStr := h.statusIcon(tx.Status)

		row := lipgloss.JoinHorizontal(lipgloss.Left,
			lipgloss.NewStyle().Width(14).Padding(0, 1).Foreground(lipgloss.Color(colorSecondary)).Render(date),
			lipgloss.NewStyle().Width(22).Padding(0, 1).Foreground(lipgloss.Color(colorWhite)).Render(counterparty),
			lipgloss.NewStyle().Width(16).Padding(0, 1).Align(lipgloss.Right).Foreground(lipgloss.Color(amountColor)).Render(amountStr),
			lipgloss.NewStyle().Width(8).Padding(0, 1).Align(lipgloss.Center).Render(statusStr),
		)

		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
			rowStyle.Render(row),
		))
		b.WriteString("\n")
	}

	// Pagination indicator.
	b.WriteString("\n")
	paginationNext := T("pagination_next", lang)
	paginationPrev := T("pagination_prev", lang)
	if h.hasMore || len(h.prevCursors) > 0 {
		pageInfo := ""
		if len(h.prevCursors) > 0 {
			pageInfo += paginationPrev
		}
		if h.hasMore && !h.loading {
			if pageInfo != "" {
				pageInfo += " • "
			}
			pageInfo += paginationNext
		}
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).
			Foreground(lipgloss.Color(colorSecondary)).Render(
				fmt.Sprintf(T("pagination_transactions", lang)+" %s", len(h.items), pageInfo),
			))
	} else {
		b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).
			Foreground(lipgloss.Color(colorSecondary)).Render(
				fmt.Sprintf(T("pagination_transactions", lang), len(h.items)),
			))
	}

	b.WriteString("\n")
	b.WriteString(HelpStyle.Render(T("help_history", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}
