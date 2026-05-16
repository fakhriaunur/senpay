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
	amount := NewTextInput("Jumlah (Rp)", true, false)
	amount.Prompt = "Rp "

	bank := NewTextInput("No. Rekening (10-16 digit)", false, false)

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
func withdrawCmd(token, idempotencyKey string, amountSen int64, bankAccount string) tea.Cmd {
	return func() tea.Msg {
		result, err := PostWithdraw(token, idempotencyKey, amountSen, bankAccount)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "Saldo tidak cukup") {
				return withdrawErrMsg{err: "Saldo tidak mencukupi"}
			}
			if strings.Contains(errMsg, "Melebihi batas transaksi") {
				return withdrawErrMsg{err: "Melebihi batas transaksi"}
			}
			if strings.Contains(errMsg, "Jumlah tidak valid") {
				return withdrawErrMsg{err: "Jumlah tidak valid"}
			}
			if strings.Contains(errMsg, "BANK_TIMEOUT") || strings.Contains(errMsg, "Bank timeout") {
				return withdrawErrMsg{err: "Bank tidak merespon, dana dikembalikan"}
			}
			if strings.Contains(errMsg, "BANK_REJECTION") || strings.Contains(errMsg, "Bank menolak") {
				return withdrawErrMsg{err: "Bank menolak permintaan tarik tunai"}
			}
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") {
				return withdrawErrMsg{err: "Gagal terhubung ke server"}
			}
			if strings.Contains(errMsg, "timeout") {
				return withdrawErrMsg{err: "Bank tidak merespon, dana dikembalikan"}
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
				w.errMsg = "Jumlah penarikan wajib diisi"
				return w, nil
			}
			amountSen, err := parseAmountSen(amountRaw)
			if err != nil || amountSen <= 0 {
				w.errMsg = "Jumlah tidak valid"
				return w, nil
			}
			if amountSen < DefaultTUIMinAmountSen {
				w.errMsg = "Minimal penarikan Rp 100"
				return w, nil
			}
			if amountSen > w.session.BalanceSen {
				w.errMsg = "Saldo tidak mencukupi"
				return w, nil
			}

			bankAccount := filterDigits(w.bankInput.Value())
			if bankAccount == "" {
				w.errMsg = "Nomor rekening wajib diisi"
				return w, nil
			}
			if len(bankAccount) < 10 || len(bankAccount) > 16 {
				w.errMsg = "Nomor rekening harus 10-16 digit"
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
			return w, withdrawCmd(w.session.Token, idempotencyKey, w.amountSen, w.bankAccount)
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

	b.WriteString(TitleStyle.Render("Tarik Tunai / Withdraw"))
	b.WriteString("\n")
	b.WriteString(SubtitleStyle.Render("Tarik saldo ke rekening bank"))
	b.WriteString("\n\n")

	// Show current balance.
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorSecondary)).
			Render(fmt.Sprintf("Saldo saat ini: %s", formatAmountSen(w.session.BalanceSen))),
	))
	b.WriteString("\n\n")

	if w.errMsg != "" {
		b.WriteString(ErrorStyle.Render(w.errMsg))
		b.WriteString("\n\n")
	}

	// Amount input.
	b.WriteString(InputPromptStyle.Render("Jumlah Penarikan (Rp)"))
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
	b.WriteString(InputPromptStyle.Render("No. Rekening Tujuan"))
	b.WriteString("\n")
	if w.focusIndex == 1 {
		b.WriteString(FocusedInputStyle.Render(w.bankInput.View()))
	} else {
		b.WriteString(InputStyle().Render(w.bankInput.View()))
	}
	b.WriteString("\n\n")

	// Submit button.
	if w.focusIndex == 2 {
		b.WriteString(FocusedButtonStyle.Render("Tarik Tunai"))
	} else {
		b.WriteString(ButtonStyle.Render("Tarik Tunai"))
	}
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render("Tab: pindah field • Enter: lanjut • Esc: kembali"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderConfirm() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Konfirmasi Penarikan"))
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
		InputPromptStyle.Render("Jumlah Penarikan"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(formatAmountSen(w.amountSen)),
		"",
		InputPromptStyle.Render("Rekening Tujuan"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(maskedAccount),
		"",
		InputPromptStyle.Render("Bank"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render("BRI"),
		"",
		InputPromptStyle.Render("Estimasi Tiba"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render("1 x 24 jam"),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confStyle.Render(content),
	))
	b.WriteString("\n\n")

	// Buttons.
	var confirmLabel string
	if w.confirmFocus == 0 {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorPrimary)).Padding(0, 2).Render("✓ Konfirmasi")
	} else {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorHighlight)).Padding(0, 2).Render("✓ Konfirmasi")
	}

	var cancelLabel string
	if w.confirmFocus == 1 {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorError)).Padding(0, 2).Render("✕ Batal")
	} else {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorMuted)).Padding(0, 2).Render("✕ Batal")
	}

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confirmLabel + "  " + cancelLabel,
	))
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render("Enter: konfirmasi • Tab: pilih • Esc: batal"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderLoading() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Tarik Tunai"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrimary)).
			Bold(true).
			Render("Memproses penarikan..."),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render("⏳"),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderSuccess() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Penarikan Berhasil"))
	b.WriteString("\n\n")

	successStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSuccess)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render("✓ Penarikan berhasil!"),
		"",
		InputPromptStyle.Render("Jumlah"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(formatAmountSen(w.amountSen)),
		"",
		InputPromptStyle.Render("Sisa Saldo"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(w.newBalance)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Render(w.createdAt),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		successStyle.Render(content),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render("Enter/Esc: kembali ke Dashboard"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (w *withdrawScreen) renderError() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Penarikan Gagal"))
	b.WriteString("\n\n")

	errorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	errContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render("✕ Penarikan gagal"),
		"",
		ErrorStyle.Render(w.errMsg),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		errorStyle.Render(errContent),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorPrimary)).Padding(0, 2).Render("Enter: Coba Lagi"),
			"  ",
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorMuted)).Padding(0, 2).Render("Esc: Kembali"),
		),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}
