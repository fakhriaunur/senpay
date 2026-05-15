package tui

// Session holds the authenticated user's session state.
type Session struct {
	Token        string
	RefreshToken string
	Phone        string
	BalanceSen   int64
	BalanceVer   int
}

// NewSession creates a new empty session.
func NewSession() *Session {
	return &Session{}
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

// Clear resets the session (logout).
func (s *Session) Clear() {
	s.Token = ""
	s.RefreshToken = ""
	s.Phone = ""
	s.BalanceSen = 0
	s.BalanceVer = 0
}
