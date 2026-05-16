package tui

import (
	"strings"

	"senpay/internal/types"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// loginScreen is the login screen model.
type loginScreen struct {
	phoneInput textinput.Model
	pinInput   textinput.Model
	focusIndex int   // 0=phone, 1=pin, 2=login button
	errMsg     string
	loading    bool
	session    *Session
}

// newLoginScreen creates a new login screen.
func newLoginScreen(session *Session) *loginScreen {
	phone := NewTextInput(session.T("phone_label")+" (08xxx)", true, false)
	phone.Validate = func(s string) error {
		// Only accept digits
		clean := strings.TrimSpace(s)
		for _, c := range clean {
			if c < '0' || c > '9' {
				return nil // just don't add to input, but don't block
			}
		}
		return nil
	}

	pin := NewTextInput(session.T("pin_label"), false, true)

	return &loginScreen{
		phoneInput: phone,
		pinInput:   pin,
		focusIndex: 0,
		session:    session,
	}
}

// loginMsg is sent when login succeeds.
type loginSuccessMsg struct {
	token        string
	refreshToken string
	phone        string
}

// loginErrMsg is sent when login fails.
type loginErrMsg struct {
	err string
}

// loginCmd performs the login API call.
func loginCmd(phone, pin, lang string) tea.Cmd {
	return func() tea.Msg {
		token, refreshToken, err := Login(phone, pin)
		if err != nil {
			// Map errors to localized messages.
			errMsg := err.Error()
			if strings.Contains(errMsg, "PIN salah") {
				return loginErrMsg{err: T("error_pin_wrong", lang)}
			}
			if strings.Contains(errMsg, "Pengguna tidak ditemukan") {
				return loginErrMsg{err: T("error_user_not_found", lang)}
			}
			if strings.Contains(errMsg, "network error") || strings.Contains(errMsg, "connection refused") || strings.Contains(errMsg, "no such host") {
				return loginErrMsg{err: T("error_network", lang)}
			}
			if strings.Contains(errMsg, "timeout") || strings.Contains(errMsg, "Timeout") {
				return loginErrMsg{err: T("error_network", lang)}
			}
			return loginErrMsg{err: errMsg}
		}
		return loginSuccessMsg{
			token:        token,
			refreshToken: refreshToken,
			phone:        phone,
		}
	}
}

func (l *loginScreen) Init() tea.Cmd {
	return textinput.Blink
}

func (l *loginScreen) Update(msg tea.Msg) (*loginScreen, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// Handle Ctrl+C globally.
		if msg.String() == "ctrl+c" {
			return l, tea.Quit
		}

		// Don't process keyboard input while loading.
		if l.loading {
			return l, nil
		}

		switch msg.String() {
		case "tab", "down":
			l.focusIndex = (l.focusIndex + 1) % 3
			l.updateFocus()
			return l, nil

		case "shift+tab", "up":
			l.focusIndex = (l.focusIndex - 1 + 3) % 3
			l.updateFocus()
			return l, nil

		case "enter":
			if l.focusIndex == 2 {
				// Submit login.
				phone := strings.TrimSpace(l.phoneInput.Value())
				pin := l.pinInput.Value()

				// Validate.
				if phone == "" {
					l.errMsg = l.session.T("error_phone_required")
					return l, nil
				}
				if pin == "" {
					l.errMsg = l.session.T("error_pin_required")
					return l, nil
				}

				// Basic phone validation.
				cleanPhone := strings.TrimPrefix(phone, "+")
				if !strings.HasPrefix(cleanPhone, types.PhonePrefix08) && !strings.HasPrefix(cleanPhone, types.PhonePrefix62) {
					l.errMsg = l.session.T("error_invalid_phone_format")
					return l, nil
				}
				if len(cleanPhone) < types.PhoneMinLength || len(cleanPhone) > TUIPhoneMaxLength {
					l.errMsg = l.session.T("error_invalid_phone_format")
					return l, nil
				}

				l.errMsg = ""
				l.loading = true
				return l, loginCmd(cleanPhone, pin, l.session.Lang())
			}
			return l, nil

		case "esc":
			// On login screen, esc does nothing special.
			return l, nil
		}

		// Let the focused input handle the key.
		var cmd tea.Cmd
		if l.focusIndex == 0 {
			// For phone input, filter non-digits.
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeyBackspace || msg.Type == tea.KeyDelete {
				l.phoneInput, cmd = l.phoneInput.Update(msg)
				// Ensure only digits in phone.
				l.phoneInput.SetValue(filterDigits(l.phoneInput.Value()))
			}
		} else if l.focusIndex == 1 {
			l.pinInput, cmd = l.pinInput.Update(msg)
		}
		return l, cmd

	case loginSuccessMsg:
		l.loading = false
		l.session.SetAuth(msg.token, msg.refreshToken, msg.phone)
		// Signal parent to transition to dashboard.
		return l, func() tea.Msg {
			return loginCompleteMsg{}
		}

	case loginErrMsg:
		l.loading = false
		l.errMsg = msg.err
		return l, nil
	}

	return l, nil
}

// loginCompleteMsg is sent to the parent to indicate login completed.
type loginCompleteMsg struct{}

func (l *loginScreen) updateFocus() {
	switch l.focusIndex {
	case 0:
		l.phoneInput.Focus()
		l.pinInput.Blur()
	case 1:
		l.phoneInput.Blur()
		l.pinInput.Focus()
	case 2:
		l.phoneInput.Blur()
		l.pinInput.Blur()
	}
}

func (l *loginScreen) View() string {
	var b strings.Builder

	lang := l.session.Lang()

	// Title.
	b.WriteString(TitleStyle.Render(T("app_title", lang)))
	b.WriteString("\n")
	b.WriteString(SubtitleStyle.Render(T("login_subtitle", lang)))
	b.WriteString("\n\n")

	// Error message.
	if l.errMsg != "" {
		b.WriteString(ErrorStyle.Render(l.errMsg))
		b.WriteString("\n\n")
	}

	// Phone input.
	b.WriteString(InputPromptStyle.Render(T("phone_label", lang)))
	b.WriteString("\n")
	if l.focusIndex == 0 {
		b.WriteString(FocusedInputStyle.Render(l.phoneInput.View()))
	} else {
		b.WriteString(InputStyle().Render(l.phoneInput.View()))
	}
	b.WriteString("\n")

	// PIN input.
	b.WriteString(InputPromptStyle.Render(T("pin_label", lang)))
	b.WriteString("\n")
	if l.focusIndex == 1 {
		b.WriteString(FocusedInputStyle.Render(l.pinInput.View()))
	} else {
		b.WriteString(InputStyle().Render(l.pinInput.View()))
	}
	b.WriteString("\n\n")

	// Login button.
	if l.loading {
		b.WriteString(ButtonStyle.Render(T("button_loading", lang)))
	} else if l.focusIndex == 2 {
		b.WriteString(FocusedButtonStyle.Render(T("button_login", lang)))
	} else {
		b.WriteString(ButtonStyle.Render(T("button_login", lang)))
	}
	b.WriteString("\n\n")

	// Help text.
	b.WriteString(HelpStyle.Render(T("help_login", lang)))

	return lipgloss.NewStyle().Width(80).Align(lipgloss.Center).Render(b.String())
}

// filterDigits removes non-digit characters from a string.
func filterDigits(s string) string {
	var result strings.Builder
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result.WriteRune(c)
		}
	}
	return result.String()
}
