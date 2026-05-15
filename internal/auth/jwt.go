package auth

import (
	"time"

	"senpay/internal/types"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	// AccessTokenDuration is the TTL for access tokens.
	AccessTokenDuration = 30 * time.Minute
	// RefreshTokenDuration is the TTL for refresh tokens.
	RefreshTokenDuration = 7 * 24 * time.Hour
)

// TokenType distinguishes access from refresh tokens.
type TokenType string

const (
	TokenTypeAccess  TokenType = "access"
	TokenTypeRefresh TokenType = "refresh"
)

// CustomClaims extends jwt.RegisteredClaims with a token type discriminator.
type CustomClaims struct {
	TokenType TokenType `json:"token_type"`
	jwt.RegisteredClaims
}

// GenerateAccessToken creates a short-lived JWT signed with HS256.
// The token carries the user UUID as the subject and expires after 30 minutes.
func GenerateAccessToken(userID uuid.UUID, secret string) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		TokenType: TokenTypeAccess,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.Must(uuid.NewV7()).String(),
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(AccessTokenDuration)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GenerateRefreshToken creates a long-lived JWT for token refresh operations.
// The token expires after 7 days. Each token has a unique JWT ID (jti) for
// single-use rotation tracking.
func GenerateRefreshToken(userID uuid.UUID, secret string) (string, error) {
	now := time.Now()
	claims := CustomClaims{
		TokenType: TokenTypeRefresh,
		RegisteredClaims: jwt.RegisteredClaims{
			ID:        uuid.Must(uuid.NewV7()).String(),
			Subject:   userID.String(),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(RefreshTokenDuration)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// ValidateToken parses a JWT, verifies the HMAC signature, and returns the claims.
// Returns ErrUnauthorized if the token is invalid, expired, or has a bad signature.
func ValidateToken(tokenString string, secret string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &CustomClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, types.ErrUnauthorized
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, types.ErrUnauthorized
	}
	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, types.ErrUnauthorized
	}
	return claims, nil
}

// ParseUserID extracts the user UUID from the JWT subject claim.
func ParseUserID(claims *CustomClaims) (uuid.UUID, error) {
	return uuid.Parse(claims.Subject)
}
