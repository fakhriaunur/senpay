package tui

import "senpay/internal/i18n"

// Session holds the authenticated user's session state.
type Session struct {
	Token        string
	RefreshToken string
	Phone        string
	BalanceSen   int64
	BalanceVer   int
	Language     string
}

// NewSession creates a new empty session.
func NewSession() *Session {
	return &Session{
		Language: i18n.DefaultLang,
	}
}

// IsAuthenticated returns true if the session has a valid token.
func (s *Session) IsAuthenticated() bool {
	return s.Token != ""
}

// SetAuth stores authentication tokens.
func (s *Session) SetAuth(token, refreshToken, phone string) {
	s.Token = token
	s.RefreshToken = refreshToken
	s.Phone = phone
}

// SetBalance updates the balance in the session.
func (s *Session) SetBalance(balanceSen int64, version int) {
	s.BalanceSen = balanceSen
	s.BalanceVer = version
}

// Lang returns the active language code.
func (s *Session) Lang() string {
	if s.Language == "" {
		return i18n.DefaultLang
	}
	return s.Language
}

// SetLang sets the active language.
func (s *Session) SetLang(lang string) {
	s.Language = lang
}

// T returns a translated string for the given key using the session's language.
func (s *Session) T(key string, args ...interface{}) string {
	return i18n.T(key, s.Lang(), args...)
}

// Clear resets the session (logout).
func (s *Session) Clear() {
	s.Token = ""
	s.RefreshToken = ""
	s.Phone = ""
	s.BalanceSen = 0
	s.BalanceVer = 0
	s.Language = i18n.DefaultLang
}

// T is a convenience function for i18n.T when no session is available.
func T(key string, lang string, args ...interface{}) string {
	return i18n.T(key, lang, args...)
}
