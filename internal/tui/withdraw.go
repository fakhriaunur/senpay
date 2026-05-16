// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// Withdraw screen states.
const (
	withdrawStateForm = iota
	withdrawStateConfirm
	withdrawStateLoading
	withdrawStateSuccess
	withdrawStateError
)

// withdrawScreen is the withdraw screen model.
type withdrawScreen struct {
	session        *Session
	state          int
	amountInput    textinput.Model
	bankInput      textinput.Model
	focusIndex     int // 0=amount, 1=bank, 2=submit button
	errMsg         string
	confirmFocus   int // 0=confirm, 1=cancel

	// Submitted values.
	amountSen    int64
	bankAccount  string

	// Result state.
	txID       string
	newBalance int64
	createdAt  string
}

// newWithdrawScreen creates a new withdraw screen.
func newWithdrawScreen(session *Session) *withdrawScreen {
	amount := NewTextInput(session.T("withdraw_amount_label"), true, false)
	amount.Prompt = "Rp "

	bank := NewTextInput(session.T("bank_account_label")+" (10-16 digit)", false, false)

	return &withdrawScreen{
		session:     session,
		state:       withdrawStateForm,
		amountInput: amount,
		bankInput:   bank,
		focusIndex:  0,
	}
}

// withdrawSubmitMsg is sent when withdraw API succeeds.
type withdrawSubmitMsg struct {
	txID       string
	amountSen  int64
	status     string
	createdAt  string
}

// withdrawErrMsg is sent when withdraw API fails.
type withdrawErrMsg struct {
	err string
}

// withdrawCmd performs the withdraw API call.
func withdrawCmd(token, idempotencyKey string, amountSen int64, bankAccount string, lang string) tea.Cmd {
	return func() tea.Msg {
		result, err := PostWithdraw(token, idempotencyKey, amountSen, bankAccount)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "Saldo tidak cukup") {
				return withdrawErrMsg{err: T("error_insufficient_balance", lang)}
			}
			if strings.Contains(errMsg, "Melebihi batas transaksi") {
				return withdrawErrMsg{err: T("error_exceeds_limit", lang)}
			}
			if strings.Contains(errMsg, "Jumlah tidak valid") {
				return withdrawErrMsg{err: T("error_invalid_amount", lang)}
			}
			if strings.Contains(errMsg, "BANK_TIMEOUT") || strings.Contains(errMsg, "Bank timeout") {
				return withdrawErrMsg{err: T("error_bank_timeout", lang)}
			}
			if strings.Contains(errMsg, "BANK_REJECTION") || strings.Contains(errMsg, "Bank menolak") {
				return withdrawErrMsg{err: T("error_bank_rejection", lang)}
			}
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") {
				return withdrawErrMsg{err: T("error_network", lang)}
			}
			if strings.Contains(errMsg, "timeout") {
				return withdrawErrMsg{err: T("error_bank_timeout", lang)}
			}
			return withdrawErrMsg{err: errMsg}
		}

		return withdrawSubmitMsg{
			txID:      result.TxID,
			amountSen: result.AmountSen,
			status:    result.Status,
			createdAt: result.CreatedAt,
		}
	}
}

func (w *withdrawScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (w *withdrawScreen) Update(msg tea.Msg) (*withdrawScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return w, tea.Quit
		}

		switch w.state {
		case withdrawStateForm:
			return w.updateForm(msg)
		case withdrawStateConfirm:
			return w.updateConfirm(msg)
		case withdrawStateLoading:
			return w, nil
		case withdrawStateSuccess:
			return w.updateSuccess(msg)
		case withdrawStateError:
			return w.updateError(msg)
		}

	case withdrawSubmitMsg:
		w.txID = msg.txID
		w.createdAt = msg.createdAt
		w.state = withdrawStateSuccess

		// Refresh balance from session after deducting.
		// The session balance will be updated on next dashboard visit.
		w.newBalance = w.session.BalanceSen - w.amountSen
		w.session.SetBalance(w.newBalance, w.session.BalanceVer+1)
		return w, nil

	case withdrawErrMsg:
		w.state = withdrawStateError
		w.errMsg = msg.err
		return w, nil
	}

	return w, nil
}

func (w *withdrawScreen) updateForm(msg tea.KeyMsg) (*withdrawScreen, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		w.focusIndex = (w.focusIndex + 1) % 3
		w.updateFormFocus()
		return w, nil

	case "shift+tab", "up":
		w.focusIndex = (w.focusIndex - 1 + 3) % 3
		w.updateFormFocus()
		return w, nil

	case "enter":
		if w.focusIndex == 2 {
			// Validate fields.
			amountRaw := filterDigits(w.amountInput.Value())

			if amountRaw == "" || amountRaw == "0" {
				w.errMsg = w.session.T("error_amount_required_withdraw")
				return w, nil
			}
			amountSen, err := parseAmountSen(amountRaw)
			if err != nil || amountSen <= 0 {
				w.errMsg = w.session.T("error_invalid_amount")
				return w, nil
			}
			if amountSen < DefaultTUIMinAmountSen {
				w.errMsg = w.session.T("error_min_withdraw")
				return w, nil
			}
			if amountSen > w.session.BalanceSen {
				w.errMsg = w.session.T("error_insufficient_balance")
				return w, nil
			}

			bankAccount := filterDigits(w.bankInput.Value())
			if bankAccount == "" {
				w.errMsg = w.session.T("error_bank_account_required")
				return w, nil
			}
			if len(bankAccount) < 10 || len(bankAccount) > 16 {
				w.errMsg = w.session.T("error_invalid_bank_account")
				return w, nil
			}

			// All valid — go to confirmation.
			w.errMsg = ""
			w.amountSen = amountSen
			w.bankAccount = bankAccount
			w.state = withdrawStateConfirm
			w.confirmFocus = 0
			return w, nil
		}
		return w, nil

	case "esc":
		return w, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}

	// Handle input fields.
	if w.focusIndex == 0 {
		var cmd tea.Cmd
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			w.amountInput, cmd = w.amountInput.Update(msg)
			w.amountInput.SetValue(filterDigits(w.amountInput.Value()))
		}
		return w, cmd
	} else if w.focusIndex == 1 {
		var cmd tea.Cmd
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			w.bankInput, cmd = w.bankInput.Update(msg)
			w.bankInput.SetValue(filterDigits(w.bankInput.Value()))
		}
		return w, cmd
	}

	return w, nil
}

func (w *withdrawScreen) updateFormFocus() {
	switch w.focusIndex {
	case 0:
		w.amountInput.Focus()
		w.bankInput.Blur()
	case 1:
		w.amountInput.Blur()
		w.bankInput.Focus()
	case 2:
		w.amountInput.Blur()
		w.bankInput.Blur()
	}
}

func (w *withdrawScreen) updateConfirm(msg tea.KeyMsg) (*withdrawScreen, tea.Cmd) {
	switch msg.String() {
	case "tab", "down", "shift+tab", "up":
		w.confirmFocus = (w.confirmFocus + 1) % 2
		return w, nil

	case "enter":
		if w.confirmFocus == 0 {
			// Confirm withdraw.
			idempotencyKey := uuid.Must(uuid.NewV7()).String()
			w.state = withdrawStateLoading
			return w, withdrawCmd(w.session.Token, idempotencyKey, w.amountSen, w.bankAccount, w.session.Lang())
		}
		// Cancel — return to form.
		w.state = withdrawStateForm
		w.focusIndex = 2
		w.errMsg = ""
		return w, nil

	case "esc":
		// Cancel — return to form.
		w.state = withdrawStateForm
		w.focusIndex = 2
		w.errMsg = ""
		return w, nil
	}
	return w, nil
}

func (w *withdrawScreen) updateSuccess(msg tea.KeyMsg) (*withdrawScreen, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		return w, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return w, nil
}

func (w *withdrawScreen) updateError(msg tea.KeyMsg) (*withdrawScreen, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Retry — go back to form.
		w.state = withdrawStateForm
		w.errMsg = ""
		w.focusIndex = 2
		return w, nil

	case "esc":
		return w, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return w, nil
}

func (w *withdrawScreen) View() string {
	switch w.state {
	case withdrawStateForm:
		return w.renderForm()
	case withdrawStateConfirm:
		return w.renderConfirm()
	case withdrawStateLoading:
		return w.renderLoading()
	case withdrawStateSuccess:
		return w.renderSuccess()
	case withdrawStateError:
		return w.renderError()
	}
	return ""
}

func (w *withdrawScreen) renderForm() string {
	var b strings.Builder
	lang := w.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_withdraw", lang)))
	b.WriteString("\n")
	b.WriteString(SubtitleStyle.Render(T("subtitle_withdraw", lang)))
	b.WriteString("\n\n")

	// Show current balance.
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary)).
			Render(fmt.Sprintf(T("current_balance_fmt", lang), formatAmountSen(w.session.BalanceSen))),
	))
	b.WriteString("\n\n")

	if w.errMsg != "" {
		b.WriteString(ErrorStyle.Render(w.errMsg))
		b.WriteString("\n\n")
	}

	// Amount input.
	b.WriteString(InputPromptStyle.Render(T("withdraw_amount_label", lang)))
	b.WriteString("\n")
	amountVal := filterDigits(w.amountInput.Value())
	amountDisplay := ""
	if amountVal != "" {
		amountDisplay = " ≈ " + displayAmount(amountVal)
	}
	if w.focusIndex == 0 {
		b.WriteString(FocusedInputStyle.Render(w.amountInput.View()) + amountDisplay)
	} else {
		b.WriteString(InputStyle().Render(w.amountInput.View()) + amountDisplay)
	}
	b.WriteString("\n")

	// Bank account input.
	b.WriteString(InputPromptStyle.Render(T("bank_account_label", lang)))
	b.WriteString("\n")
	if w.focusIndex == 1 {
		b.WriteString(FocusedInputStyle.Render(w.bankInput.View()))
	} else {
		b.WriteString(InputStyle().Render(w.bankInput.View()))
	}
	b.WriteString("\n\n")

	// Submit button.
	if w.focusIndex == 2 {
		b.WriteString(FocusedButtonStyle.Render(T("button_withdraw", lang)))
	} else {
		b.WriteString(ButtonStyle.Render(T("button_withdraw", lang)))
	}
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render(T("help_withdraw_form", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderConfirm() string {
	var b strings.Builder
	lang := w.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_confirm_withdraw", lang)))
	b.WriteString("\n\n")

	confStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	// Mask bank account — show last 4 digits.
	maskedAccount := "••••" + w.bankAccount[len(w.bankAccount)-4:]

	content := lipgloss.JoinVertical(lipgloss.Center,
		InputPromptStyle.Render(T("withdraw_amount_label", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(formatAmountSen(w.amountSen)),
		"",
		InputPromptStyle.Render(T("label_bank_account", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(maskedAccount),
		"",
		InputPromptStyle.Render(T("label_bank_name", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(T("bank_name_bri", lang)),
		"",
		InputPromptStyle.Render(T("label_estimate", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render(T("estimate_time", lang)),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confStyle.Render(content),
	))
	b.WriteString("\n\n")

	// Buttons.
	confirmText := T("button_confirm", lang)
	cancelText := T("button_cancel", lang)
	var confirmLabel string
	if w.confirmFocus == 0 {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorPrimary)).Padding(0, 2).Render(confirmText)
	} else {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorHighlight)).Padding(0, 2).Render(confirmText)
	}

	var cancelLabel string
	if w.confirmFocus == 1 {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorError)).Padding(0, 2).Render(cancelText)
	} else {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorMuted)).Padding(0, 2).Render(cancelText)
	}

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confirmLabel + "  " + cancelLabel,
	))
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render(T("help_confirm_withdraw", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderLoading() string {
	var b strings.Builder
	lang := w.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_withdraw", lang)))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrimary)).
			Bold(true).
			Render(T("loading_withdraw", lang)),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render("⏳"),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderSuccess() string {
	var b strings.Builder
	lang := w.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_success_withdraw", lang)))
	b.WriteString("\n\n")

	successStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSuccess)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(T("success_withdraw", lang)),
		"",
		InputPromptStyle.Render(T("label_amount", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(formatAmountSen(w.amountSen)),
		"",
		InputPromptStyle.Render(T("label_remaining_balance", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(w.newBalance)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Render(w.createdAt),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		successStyle.Render(content),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render(T("help_success", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderError() string {
	var b strings.Builder
	lang := w.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_error_withdraw", lang)))
	b.WriteString("\n\n")

	errorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	errContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(T("error_withdraw", lang)),
		"",
		ErrorStyle.Render(w.errMsg),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		errorStyle.Render(errContent),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorPrimary)).Padding(0, 2).Render(T("button_retry", lang)),
			"  ",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorMuted)).Padding(0, 2).Render(T("button_back", lang)),
		),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}
