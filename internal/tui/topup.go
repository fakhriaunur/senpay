// Package tui provides the Bubble Tea terminal UI for Senpay.
//
// FCIS: Imperative Shell — manages user interaction, HTTP calls,
// screen transitions. No business logic.
package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/google/uuid"
)

// Top-up screen states.
const (
	topupStateForm = iota
	topupStateLoading
	topupStateVADisplay
	topupStateSuccess
	topupStateError
)

// DefaultVADisplayTTL is the fallback TTL for VA display when not provided by API.
const DefaultVADisplayTTL = 1 * time.Hour

// CountdownTickInterval is the interval for the VA countdown timer.
const CountdownTickInterval = 1 * time.Second

// CountdownUrgentMinutes is the minute threshold for urgent countdown coloring.
const CountdownUrgentMinutes = "30"

// CountdownCriticalMinutes is the minute threshold for critical countdown coloring.
const CountdownCriticalMinutes = "10"

// Payment methods for top-up.
var topupMethods = []string{
	"Virtual Account (BRI)",
	"Bank Transfer (BCA)",
}

// topupScreen is the top-up screen model.
type topupScreen struct {
	session     *Session
	state       int
	amountInput textinput.Model
	methodIndex int // selected payment method index
	focusIndex  int // 0=amount, 1=method, 2=submit button
	errMsg      string

	// VA display state.
	vaNumber     string
	expiresAt    time.Time
	txID         string
	createdAt    string
	amountSen    int64

	// Success state.
	newBalance int64
}

// newTopupScreen creates a new top-up screen.
func newTopupScreen(session *Session) *topupScreen {
	amount := NewTextInput(session.T("amount_label"), true, false)
	amount.Prompt = "Rp "

	return &topupScreen{
		session:     session,
		state:       topupStateForm,
		amountInput: amount,
		methodIndex: 0,
		focusIndex:  0,
	}
}

// topupSubmitMsg is sent when top-up API succeeds.
type topupSubmitMsg struct {
	txID      string
	vaNumber  string
	amountSen int64
	expiresAt time.Time
	createdAt string
}

// topupErrMsg is sent when top-up API fails.
type topupErrMsg struct {
	err string
}

// topupTickMsg ticks every second for countdown.
type topupTickMsg struct{}

// topupCmd performs the top-up API call.
func topupCmd(token, idempotencyKey string, amountSen int64, lang string) tea.Cmd {
	return func() tea.Msg {
		result, err := PostTopup(token, idempotencyKey, amountSen)
		if err != nil {
			errMsg := err.Error()
			if strings.Contains(errMsg, "Melebihi batas transaksi") {
				return topupErrMsg{err: T("error_exceeds_limit", lang)}
			}
			if strings.Contains(errMsg, "Jumlah tidak valid") {
				return topupErrMsg{err: T("error_invalid_amount", lang)}
			}
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") {
				return topupErrMsg{err: T("error_network", lang)}
			}
			if strings.Contains(errMsg, "timeout") {
				return topupErrMsg{err: T("error_timeout", lang)}
			}
			return topupErrMsg{err: errMsg}
		}

		expiresAt, _ := time.Parse(time.RFC3339Nano, result.ExpiresAt)
		if expiresAt.IsZero() {
			expiresAt = time.Now().Add(DefaultVADisplayTTL)
		}

		return topupSubmitMsg{
			txID:      result.TxID,
			vaNumber:  result.VANumber,
			amountSen: result.AmountSen,
			expiresAt: expiresAt,
			createdAt: result.CreatedAt,
		}
	}
}

// countdownTick creates a tick for countdown timer (every second).
func countdownTick() tea.Cmd {
	return tea.Tick(CountdownTickInterval, func(t time.Time) tea.Msg {
		return topupTickMsg{}
	})
}

func (t *topupScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (t *topupScreen) Update(msg tea.Msg) (*topupScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return t, tea.Quit
		}

		switch t.state {
		case topupStateForm:
			return t.updateForm(msg)
		case topupStateLoading:
			return t, nil
		case topupStateVADisplay:
			return t.updateVADisplay(msg)
		case topupStateSuccess:
			return t.updateSuccess(msg)
		case topupStateError:
			return t.updateError(msg)
		}

	case topupSubmitMsg:
		t.vaNumber = msg.vaNumber
		t.amountSen = msg.amountSen
		t.expiresAt = msg.expiresAt
		t.txID = msg.txID
		t.createdAt = msg.createdAt
		t.state = topupStateVADisplay
		return t, countdownTick()

	case topupErrMsg:
		t.state = topupStateError
		t.errMsg = msg.err
		return t, nil

	case topupTickMsg:
		// Only useful in VA display state — triggers re-render for countdown.
		if t.state == topupStateVADisplay {
			return t, countdownTick()
		}
		return t, nil
	}

	return t, nil
}

func (t *topupScreen) updateForm(msg tea.KeyMsg) (*topupScreen, tea.Cmd) {
	switch msg.String() {
	case "tab", "down":
		t.focusIndex = (t.focusIndex + 1) % 3
		t.updateFormFocus()
		return t, nil

	case "shift+tab", "up":
		t.focusIndex = (t.focusIndex - 1 + 3) % 3
		t.updateFormFocus()
		return t, nil

	case "left":
		if t.focusIndex == 1 {
			t.methodIndex = (t.methodIndex - 1 + len(topupMethods)) % len(topupMethods)
		}
		return t, nil

	case "right":
		if t.focusIndex == 1 {
			t.methodIndex = (t.methodIndex + 1) % len(topupMethods)
		}
		return t, nil

	case "enter":
		if t.focusIndex == 2 {
			// Validate and submit.
			amountRaw := filterDigits(t.amountInput.Value())
			if amountRaw == "" || amountRaw == "0" {
				t.errMsg = t.session.T("error_amount_required")
				return t, nil
			}
			amountSen, err := parseAmountSen(amountRaw)
			if err != nil || amountSen <= 0 {
				t.errMsg = t.session.T("error_invalid_amount")
				return t, nil
			}
			if amountSen < DefaultTUIMinAmountSen {
				t.errMsg = t.session.T("error_min_topup")
				return t, nil
			}

			t.errMsg = ""
			t.amountSen = amountSen
			t.state = topupStateLoading
			idempotencyKey := uuid.Must(uuid.NewV7()).String()
			return t, topupCmd(t.session.Token, idempotencyKey, amountSen, t.session.Lang())
		}
		return t, nil

	case "esc":
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}

	// Handle amount input.
	if t.focusIndex == 0 {
		var cmd tea.Cmd
		if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
			t.amountInput, cmd = t.amountInput.Update(msg)
			t.amountInput.SetValue(filterDigits(t.amountInput.Value()))
		}
		return t, cmd
	}

	return t, nil
}

func (t *topupScreen) updateFormFocus() {
	switch t.focusIndex {
	case 0:
		t.amountInput.Focus()
	case 1, 2:
		t.amountInput.Blur()
	}
}

func (t *topupScreen) updateVADisplay(msg tea.KeyMsg) (*topupScreen, tea.Cmd) {
	switch msg.String() {
	case "enter", " ", "c":
		// "Copy" action — simulate copy instruction.
		_ = t.vaNumber
		// We don't have clipboard access, just show instruction.
		return t, nil

	case "esc":
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}

	case "tab", "shift+tab":
		// Toggle between "Copy VA" button and "Back" button.
		return t, nil
	}
	return t, nil
}

func (t *topupScreen) updateSuccess(msg tea.KeyMsg) (*topupScreen, tea.Cmd) {
	switch msg.String() {
	case "enter", "esc":
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return t, nil
}

func (t *topupScreen) updateError(msg tea.KeyMsg) (*topupScreen, tea.Cmd) {
	switch msg.String() {
	case "enter":
		// Retry — go back to form with fields preserved.
		t.state = topupStateForm
		t.errMsg = ""
		t.focusIndex = 2
		return t, nil

	case "esc":
		return t, func() tea.Msg {
			return navigateToDashboardMsg{}
		}
	}
	return t, nil
}

// remainingTime returns the remaining time as a formatted string.
func (t *topupScreen) remainingTime() string {
	rem := time.Until(t.expiresAt)
	if rem <= 0 {
		return "00:00:00"
	}
	h := int(rem.Hours())
	m := int(rem.Minutes()) % 60
	s := int(rem.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", h, m, s)
}

func (t *topupScreen) View() string {
	switch t.state {
	case topupStateForm:
		return t.renderForm()
	case topupStateLoading:
		return t.renderLoading()
	case topupStateVADisplay:
		return t.renderVADisplay()
	case topupStateSuccess:
		return t.renderSuccess()
	case topupStateError:
		return t.renderError()
	}
	return ""
}

func (t *topupScreen) renderForm() string {
	var b strings.Builder
	lang := t.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_topup", lang)))
	b.WriteString("\n")
	b.WriteString(SubtitleStyle.Render(T("subtitle_topup", lang)))
	b.WriteString("\n\n")

	if t.errMsg != "" {
		b.WriteString(ErrorStyle.Render(t.errMsg))
		b.WriteString("\n\n")
	}

	// Amount input.
	b.WriteString(InputPromptStyle.Render(T("amount_label", lang)))
	b.WriteString("\n")
	amountVal := filterDigits(t.amountInput.Value())
	amountDisplay := ""
	if amountVal != "" {
		amountDisplay = " ≈ " + displayAmount(amountVal)
	}
	if t.focusIndex == 0 {
		b.WriteString(FocusedInputStyle.Render(t.amountInput.View()) + amountDisplay)
	} else {
		b.WriteString(InputStyle().Render(t.amountInput.View()) + amountDisplay)
	}
	b.WriteString("\n\n")

	// Payment method selector.
	b.WriteString(InputPromptStyle.Render(T("label_payment_method", lang)))
	b.WriteString("\n")
	var methods []string
	methodKeys := []string{"method_va_bri", "method_bank_bca"}
	for i, methodKey := range methodKeys {
		method := T(methodKey, lang)
		if i == t.methodIndex {
			selected := "◉"
			if t.focusIndex == 1 {
				methods = append(methods, lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color(colorPrimary)).
					Render(fmt.Sprintf("  %s %s", selected, method)))
			} else {
				methods = append(methods, lipgloss.NewStyle().
					Foreground(lipgloss.Color(colorPrimary)).
					Render(fmt.Sprintf("  %s %s", selected, method)))
			}
		} else {
			methods = append(methods, lipgloss.NewStyle().
				Foreground(lipgloss.Color(colorSecondary)).
				Render(fmt.Sprintf("  ○ %s", method)))
		}
	}

	methodBox := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSecondary)).
		Padding(0, 2).
		Width(36)
	if t.focusIndex == 1 {
		methodBox = methodBox.BorderForeground(lipgloss.Color(colorPrimary))
	}
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		methodBox.Render(lipgloss.JoinVertical(lipgloss.Left, methods...)),
	))
	b.WriteString("\n\n")

	// Submit button.
	if t.focusIndex == 2 {
		b.WriteString(FocusedButtonStyle.Render(T("button_topup", lang)))
	} else {
		b.WriteString(ButtonStyle.Render(T("button_topup", lang)))
	}
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render(T("help_topup", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *topupScreen) renderLoading() string {
	var b strings.Builder
	lang := t.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_topup", lang)))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().
			Foreground(lipgloss.Color(colorPrimary)).
			Bold(true).
			Render(T("loading_topup", lang)),
	))
	b.WriteString("\n\n")
	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render("⏳"),
	))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *topupScreen) renderVADisplay() string {
	var b strings.Builder
	lang := t.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_va", lang)))
	b.WriteString("\n\n")

	vaStyle := lipgloss.NewStyle().
		Border(lipgloss.DoubleBorder()).
		BorderForeground(lipgloss.Color(colorPrimary)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	// Build VA display content.
	bankName := "BRI"
	if t.methodIndex == 1 {
		bankName = "BCA"
	}

	// Large VA number display.
	vaLargeStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorPrimary)).
		Background(lipgloss.Color(colorCardBg)).
		Padding(0, 2).
		Width(40).
		Align(lipgloss.Center)

	// Format VA number with spaces for readability.
	vaDisplay := t.vaNumber
	if len(vaDisplay) >= 10 {
		vaDisplay = vaDisplay[:4] + " " + vaDisplay[4:7] + " " + vaDisplay[7:]
	}

	// Expiration countdown.
	remaining := t.remainingTime()
	countdownColor := colorSuccess
	if strings.HasPrefix(remaining, "00:") {
		m := remaining[3:5]
		if m < CountdownCriticalMinutes {
			countdownColor = colorError
		} else if m < CountdownUrgentMinutes {
			countdownColor = colorSenpai
		}
	}

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render(fmt.Sprintf(T("label_bank", lang), bankName)),
		"",
		InputPromptStyle.Render(T("label_va_number", lang)),
		vaLargeStyle.Render(vaDisplay),
		"",
		lipgloss.NewStyle().Italic(true).Foreground(lipgloss.Color(colorMuted)).
			Render(T("va_instruction", lang)),
		"",
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSecondary)).Render(fmt.Sprintf("%s: %s", T("label_amount", lang), formatAmountSen(t.amountSen))),
		"",
		lipgloss.JoinHorizontal(lipgloss.Center,
			lipgloss.NewStyle().Foreground(lipgloss.Color(colorMuted)).Render(T("va_expires", lang)+" "),
			lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(countdownColor)).Render(remaining),
		),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		vaStyle.Render(content),
	))
	b.WriteString("\n\n")

	// Action buttons.
	copyStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorPrimary)).
		Padding(0, 3).
		Width(22).
		Align(lipgloss.Center)
	backStyle := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color(colorWhite)).
		Background(lipgloss.Color(colorMuted)).
		Padding(0, 3).
		Width(22).
		Align(lipgloss.Center)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		lipgloss.JoinHorizontal(lipgloss.Center,
			copyStyle.Render(T("button_copy_va", lang)),
			"  ",
			backStyle.Render(T("button_back_va", lang)),
		),
	))
	b.WriteString("\n\n")

	b.WriteString(HelpStyle.Render(T("help_va", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *topupScreen) renderSuccess() string {
	var b strings.Builder
	lang := t.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_success_topup", lang)))
	b.WriteString("\n\n")

	successStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorSuccess)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	content := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(T("success_topup", lang)),
		"",
		InputPromptStyle.Render(T("label_amount", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(t.amountSen)),
		"",
		InputPromptStyle.Render(T("label_new_balance", lang)),
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorSuccess)).Bold(true).Render(formatAmountSen(t.newBalance)),
	)

	b.WriteString(lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(
		successStyle.Render(content),
	))
	b.WriteString("\n\n")
	b.WriteString(HelpStyle.Render(T("help_success", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

func (t *topupScreen) renderError() string {
	var b strings.Builder
	lang := t.session.Lang()

	b.WriteString(TitleStyle.Render(T("title_error_topup", lang)))
	b.WriteString("\n\n")

	errorStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color(colorError)).
		Padding(1, 3).
		Width(50).
		Align(lipgloss.Center)

	errContent := lipgloss.JoinVertical(lipgloss.Center,
		lipgloss.NewStyle().Foreground(lipgloss.Color(colorError)).Bold(true).Render(T("error_topup", lang)),
		"",
		ErrorStyle.Render(t.errMsg),
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
