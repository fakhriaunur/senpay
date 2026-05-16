// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"strings"
	"time"

	"senpay/internal/types"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// transferScreen states.
const (
	transferStateForm = iota
	transferStateConfirm
	transferStateLoading
	transferStateSuccess
	transferStateError
)

// transferScreen is the transfer screen model.
type transferScreen struct {
	session      *Session
	state        int
	phoneInput   textinput.Model
	amountInput  textinput.Model
	focusIndex   int // 0=phone, 1=amount, 2=confirm button
	errMsg       string
	txID         string
	amountSen    int64
	feeSen       int64
	newBalance   int64
	confirmPhone string
	confirmAmt   int64
	createdAt    string
}

// newTransferScreen creates a new transfer screen.
func newTransferScreen(session *Session) *transferScreen {
	phone := NewTextInput("Nomor HP penerima (08xxx)", true, false)

	amount := NewTextInput("Jumlah (Rp)", false, false)
	amount.Prompt = "Rp "

	return &transferScreen{
		session:     session,
		state:       transferStateForm,
		phoneInput:  phone,
		amountInput: amount,
		focusIndex:  0,
	}
}

// transferSubmitMsg is sent when transfer API succeeds.
type transferSubmitMsg struct {
	txID       string
	amountSen  int64
	feeSen     int64
	newBalance int64
	createdAt  string
}

// transferErrMsg is sent when transfer API fails.
type transferErrMsg struct {
	err string
}

// transferReturnTick is a tick for auto-return to dashboard.
type transferReturnTick struct{}

// transferCmd performs the transfer API call.
func transferCmd(token, idempotencyKey, toPhone string, amountSen int64) tea.Cmd {
	return func() tea.Msg {
		result, err := PostTransfer(token, idempotencyKey, toPhone, amountSen)
		if err != nil {
			errMsg := err.Error()
			// Map API errors to Indonesian-friendly messages.
			if strings.Contains(errMsg, "Saldo tidak cukup") {
				return transferErrMsg{err: "Saldo tidak mencukupi"}
			}
			if strings.Contains(errMsg, "Pengguna tidak ditemukan") {
				return transferErrMsg{err: "Pengguna tidak ditemukan"}
			}
			if strings.Contains(errMsg, "Tidak bisa transfer ke diri sendiri") {
				return transferErrMsg{err: "Tidak bisa transfer ke diri sendiri"}
			}
			if strings.Contains(errMsg, "Jumlah tidak valid") {
				return transferErrMsg{err: "Jumlah tidak valid"}
			}
			if strings.Contains(errMsg, "Melebihi batas transaksi") {
				return transferErrMsg{err: "Melebihi batas transaksi"}
			}
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") {
				return transferErrMsg{err: "Gagal terhubung ke server"}
			}
			return transferErrMsg{err: errMsg}
		}

		return transferSubmitMsg{
			txID:       result.TxID,
			amountSen:  result.AmountSen,
			feeSen:     result.FeeSen,
			newBalance: result.SenderBalanceSen,
			createdAt:  time.Now().Format("02/01/2006 15:04:05"),
		}
	}
}

// TransferAutoReturnDelay is the delay before auto-returning after a successful transfer.
const TransferAutoReturnDelay = 3 * time.Second

// returnTick creates a tick for auto-return after transfer success.
func transferReturnTickCmd() tea.Cmd {
	return tea.Tick(TransferAutoReturnDelay, func(t time.Time) tea.Msg {
		return transferReturnTick{}
	})
}

func (t *transferScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (t *transferScreen) Update(msg tea.Msg) (*transferScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return t, tea.Quit
		}

		switch t.state {
		case transferStateForm:
			return t.updateForm(msg)
		case transferStateConfirm:
			return t.updateConfirm(msg)
		case transferStateLoading:
			// Block input while loading.
			return t, nil
		case transferStateSuccess:
			return t.updateSuccess(msg)
		case transferStateError:
			return t.updateError(msg)
		}

	case transferSubmitMsg:
		t.state = transferStateSuccess
		t.txID = msg.txID
		t.amountSen = msg.amountSen
		t.feeSen = msg.feeSen
		t.newBalance = msg.newBalance
		t.createdAt = msg.createdAt
		// Update session balance.
		t.session.SetBalance(msg.newBalance, t.session.BalanceVer+1)
		return t, transferReturnTickCmd()

	case transferErrMsg:
		t.state = transferStateError
		t.errMsg = msg.err
		return t, nil

	case transferReturnTick:
		// Send signal to parent to navigate back to dashboard.
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}

	return t, nil
}

func (t *transferScreen) updateForm(msg tea.KeyMsg) (*transferScreen, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		t.focusIndex = (t.focusIndex + 1) % 3
		t.updateInputFocus()
		return t, nil

	case "shift+tab", "up":
		t.focusIndex = (t.focusIndex - 1 + 3) % 3
		t.updateInputFocus()
		return t, nil

	case "enter":
		if t.focusIndex == 2 {
			// Validate and move to confirmation.
			phone := filterDigits(t.phoneInput.Value())
			amountRaw := filterDigits(t.amountInput.Value())

			// Validate phone.
			if phone == "" {
				t.errMsg = "Nomor HP penerima wajib diisi"
				return t, nil
			}
			cleanPhone := strings.TrimPrefix(phone, "+")
			if !strings.HasPrefix(cleanPhone, types.PhonePrefix08) && !strings.HasPrefix(cleanPhone, types.PhonePrefix62) {
				t.errMsg = "Format nomor HP tidak valid"
				return t, nil
			}
			if len(cleanPhone) < types.PhoneMinLength || len(cleanPhone) > TUIPhoneMaxLength {
				t.errMsg = "Format nomor HP tidak valid"
				return t, nil
			}

			// Validate amount.
			if amountRaw == "" || amountRaw == "0" {
				t.errMsg = "Jumlah transfer wajib diisi"
				return t, nil
			}
			amountSen, err := parseAmountSen(amountRaw)
			if err != nil || amountSen <= 0 {
				t.errMsg = "Jumlah tidak valid"
				return t, nil
			}
			if amountSen < 1000 {
				t.errMsg = "Minimal transfer Rp 10"
				return t, nil
			}

			t.errMsg = ""
			t.confirmPhone = phone
			t.confirmAmt = amountSen
			t.state = transferStateConfirm
			t.focusIndex = 0 // focus the "Confirm" button
			return t, nil
		}
		return t, nil

	case "esc":
		// Esc returns to dashboard from form.
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}

	// Handle input fields.
	var cmd tea.Cmd
	if t.focusIndex == 0 {
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			t.phoneInput, cmd = t.phoneInput.Update(msg)
			t.phoneInput.SetValue(filterDigits(t.phoneInput.Value()))
		}
	} else if t.focusIndex == 1 {
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			t.amountInput, cmd = t.amountInput.Update(msg)
			t.amountInput.SetValue(filterDigits(t.amountInput.Value()))
		}
	}
	return t, cmd
}

func (t *transferScreen) updateConfirm(msg tea.KeyMsg) (*transferScreen, tea.Cmd) {
	switch msg.String() {
	case "tab", "down", "shift+tab", "up":
		t.focusIndex = (t.focusIndex + 1) % 2
		return t, nil

	case "enter":
		if t.focusIndex == 0 {
			// Confirm transfer — generate idempotency key and submit.
			idempotencyKey := uuid.Must(uuid.NewV7()).String()
			t.state = transferStateLoading
			t.errMsg = ""
			return t, transferCmd(t.session.Token, idempotencyKey, t.confirmPhone, t.confirmAmt)
		}
		// Cancel — return to form.
		t.state = transferStateForm
		t.focusIndex = 2 // focus submit button
		return t, nil

	case "esc":
		// Cancel — return to form.
		t.state = transferStateForm
		t.focusIndex = 2
		return t, nil
	}
	return t, nil
}

func (t *transferScreen) updateSuccess(msg tea.KeyMsg) (*transferScreen, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		// Return to dashboard immediately.
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return t, nil
}

func (t *transferScreen) updateError(msg tea.KeyMsg) (*transferScreen, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Retry — go back to form with fields preserved.
		t.state = transferStateForm
		t.errMsg = ""
		t.focusIndex = 2
		return t, nil

	case "esc":
		// Return to dashboard.
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return t, nil
}

func (t *transferScreen) updateInputFocus() {
	switch t.focusIndex {
	case 0:
		t.phoneInput.Focus()
		t.amountInput.Blur()
	case 1:
		t.phoneInput.Blur()
		t.amountInput.Focus()
	case 2:
		t.phoneInput.Blur()
		t.amountInput.Blur()
	}
}

// formatAmountSen formats a sen amount as display IDR string.
func formatAmountSen(sen int64) string {
	return formatIDR(sen)
}

// parseAmountSen parses a digit string into sen (multiply by 100 for IDR).
func parseAmountSen(digits string) (int64, error) {
	if digits == "" {
		return 0, fmt.Errorf("empty")
	}
	// User enters amount in IDR (e.g., "50000" = Rp 50.000).
	// Convert to sen by multiplying by 100.
	idr := int64(0)
	for _, c := range digits {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid digit")
		}
		idr = idr*10 + int64(c-'0')
	}
	return idr * 100, nil
}

// displayAmount formats raw digit string for display as IDR.
func displayAmount(digits string) string {
	if digits == "" {
		return ""
	}
	idr := int64(0)
	for _, c := range digits {
		if c < '0' || c > '9' {
			continue
		}
		idr = idr*10 + int64(c-'0')
	}
	return formatIDR(idr * 100)
}

func (t *transferScreen) View() string {
	switch t.state {
	case transferStateForm:
		return t.renderForm()
	case transferStateConfirm:
		return t.renderConfirm()
	case transferStateLoading:
		return t.renderLoading()
	case transferStateSuccess:
		return t.renderSuccess()
	case transferStateError:
		return t.renderError()
	}
	return ""
}

func (t *transferScreen) renderForm() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Transfer"))
	b.WriteString("\n")
	b.WriteString(SubtitleStyle.Render("Kirim uang ke pengguna lain"))
	b.WriteString("\n\n")

	if t.errMsg != "" {
		b.WriteString(ErrorStyle.Render(t.errMsg))
		b.WriteString("\n\n")
	}

	// Phone input.
	b.WriteString(InputPromptStyle.Render("Nomor HP Penerima"))
	b.WriteString("\n")
	if t.focusIndex == 0 {
		b.WriteString(FocusedInputStyle.Render(t.phoneInput.View()))
	} else {
		b.WriteString(InputStyle().Render(t.phoneInput.View()))
	}
	b.WriteString("\n")

	// Amount input.
	b.WriteString(InputPromptStyle.Render("Jumlah (Rp)"))
	b.WriteString("\n")
	// Show live preview of formatted amount.
	amountVal := filterDigits(t.amountInput.Value())
	amountDisplay := ""
	if amountVal != "" {
		amountDisplay = " ≈ " + displayAmount(amountVal)
	}
	if t.focusIndex == 1 {
		b.WriteString(FocusedInputStyle.Render(t.amountInput.View()) + amountDisplay)
	} else {
		b.WriteString(InputStyle().Render(t.amountInput.View()) + amountDisplay)
	}
	b.WriteString("\n\n")

	// Submit button.
	if t.focusIndex == 2 {
		b.WriteString(FocusedButtonStyle.Render("Transfer"))
	} else {
		b.WriteString(ButtonStyle.Render("Transfer"))
	}
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render("Tab: pindah field • Enter: lanjut • Esc: kembali"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *transferScreen) renderConfirm() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Konfirmasi Transfer"))
	b.WriteString("\n\n")

	confStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	content := lipgloss.JoinVertical(lipgloss.Center,
		InputPromptStyle.Render("Penerima"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(t.confirmPhone),
		"",
		InputPromptStyle.Render("Jumlah"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(t.confirmAmt)),
		"",
		InputPromptStyle.Render("Saldo saat ini"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(formatAmountSen(t.session.BalanceSen)),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confStyle.Render(content),
	))
	b.WriteString("\n\n")

	// Buttons.
	var confirmLabel string
	if t.focusIndex == 0 {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorPrimary)).Padding(0, 2).Render("✓ Kirim")
	} else {
		confirmLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorHighlight)).Padding(0, 2).Render("✓ Kirim")
	}

	var cancelLabel string
	if t.focusIndex == 1 {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorError)).Padding(0, 2).Render("✕ Batal")
	} else {
		cancelLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(colorWhite)).Background(lipgloss.Color(colorMuted)).Padding(0, 2).Render("✕ Batal")
	}

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		confirmLabel + "  " + cancelLabel,
	))
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render("Enter: konfirmasi • Esc: batal"))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *transferScreen) renderLoading() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Transfer"))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrimary)).
			Bold(true).
			Render("Memproses transfer..."),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render("⏳"),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *transferScreen) renderSuccess() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Transfer Berhasil"))
	b.WriteString("\n\n")

	successStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSuccess)).
		Padding(1, 3).
		Width(54).
		Align(lipgloss.Center)

	successContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render("✓ Transfer berhasil!"),
		"",
		InputPromptStyle.Render("ID Transaksi"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorWhite)).Render(shortID(t.txID)),
		"",
		InputPromptStyle.Render("Jumlah"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(t.amountSen)),
		"",
		InputPromptStyle.Render("Biaya"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render(formatAmountSen(t.feeSen)),
		"",
		InputPromptStyle.Render("Saldo Baru"),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(t.newBalance)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Render(t.createdAt),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		successStyle.Render(successContent),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render("Kembali ke Dashboard dalam 3 detik..."))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *transferScreen) renderError() string {
	var b strings.Builder

	b.WriteString(TitleStyle.Render("Transfer Gagal"))
	b.WriteString("\n\n")

	errorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	errContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render("✕ Transfer gagal"),
		"",
		ErrorStyle.Render(t.errMsg),
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

// shortID returns a shortened version of a UUID for display.
func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8] + "..."
	}
	return id
}
