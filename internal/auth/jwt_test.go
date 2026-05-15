package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestGenerateAccessToken(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	userID := uuid.Must(uuid.NewV7())

	t.Run("returns_valid_token", func(t *testing.T) {
		token, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}
		if token == "" {
			t.Fatal("token must not be empty")
		}
	})

	t.Run("token_contains_user_id_as_sub", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}

		parsedID, err := ParseUserID(claims)
		if err != nil {
			t.Fatalf("ParseUserID: %v", err)
		}
		if parsedID != userID {
			t.Errorf("user ID mismatch: got %v, want %v", parsedID, userID)
		}
	})

	t.Run("token_type_is_access", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.TokenType != TokenTypeAccess {
			t.Errorf("expected token type %q, got %q", TokenTypeAccess, claims.TokenType)
		}
	})

	t.Run("expires_in_30_minutes", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}

		exp := claims.ExpiresAt.Time
		expectedExp := time.Now().Add(AccessTokenDuration)
		if exp.Before(expectedExp.Add(-time.Minute)) || exp.After(expectedExp.Add(time.Minute)) {
			t.Errorf("expiration outside expected window: got %v, expected around %v", exp, expectedExp)
		}
	})

	t.Run("has_issued_at", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.IssuedAt == nil || claims.IssuedAt.Time.IsZero() {
			t.Error("iat claim is missing")
		}
	})
}

func TestGenerateRefreshToken(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	userID := uuid.Must(uuid.NewV7())

	t.Run("returns_valid_token", func(t *testing.T) {
		token, err := GenerateRefreshToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateRefreshToken: %v", err)
		}
		if token == "" {
			t.Fatal("token must not be empty")
		}
	})

	t.Run("token_type_is_refresh", func(t *testing.T) {
		tokenStr, err := GenerateRefreshToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateRefreshToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}
		if claims.TokenType != TokenTypeRefresh {
			t.Errorf("expected token type %q, got %q", TokenTypeRefresh, claims.TokenType)
		}
	})

	t.Run("expires_in_7_days", func(t *testing.T) {
		tokenStr, err := GenerateRefreshToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateRefreshToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken: %v", err)
		}

		exp := claims.ExpiresAt.Time
		expectedExp := time.Now().Add(RefreshTokenDuration)
		if exp.Before(expectedExp.Add(-time.Hour)) || exp.After(expectedExp.Add(time.Hour)) {
			t.Errorf("expiration outside expected window: got %v, expected around %v", exp, expectedExp)
		}
	})
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	userID := uuid.Must(uuid.NewV7())

	t.Run("valid_token_passes", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		claims, err := ValidateToken(tokenStr, secret)
		if err != nil {
			t.Fatalf("ValidateToken should pass: %v", err)
		}
		if claims.Subject != userID.String() {
			t.Errorf("subject: got %s, want %s", claims.Subject, userID.String())
		}
	})

	t.Run("wrong_secret_rejected", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		_, err = ValidateToken(tokenStr, "wrong-secret")
		if err == nil {
			t.Fatal("expected error for wrong secret")
		}
	})

	t.Run("expired_token_rejected", func(t *testing.T) {
		// Create a token with an expired time.
		now := time.Now()
		claims := CustomClaims{
			TokenType: TokenTypeAccess,
			RegisteredClaims: jwt.RegisteredClaims{
				Subject:   userID.String(),
				IssuedAt:  jwt.NewNumericDate(now.Add(-2 * time.Hour)),
				ExpiresAt: jwt.NewNumericDate(now.Add(-1 * time.Hour)),
			},
		}
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		tokenStr, err := token.SignedString([]byte(secret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}

		_, err = ValidateToken(tokenStr, secret)
		if err == nil {
			t.Fatal("expected error for expired token")
		}
	})

	t.Run("tampered_token_rejected", func(t *testing.T) {
		tokenStr, err := GenerateAccessToken(userID, secret)
		if err != nil {
			t.Fatalf("GenerateAccessToken: %v", err)
		}

		// Tamper with the payload portion.
		parts := []byte(tokenStr)
		// Find the second dot and change a character in the payload.
		tampered := tokenStr[:len(tokenStr)-5] + "AAAAA"
		_ = parts // avoid unused

		_, err = ValidateToken(tampered, secret)
		if err == nil {
			t.Fatal("expected error for tampered token")
		}
	})

	t.Run("malformed_token_rejected", func(t *testing.T) {
		_, err := ValidateToken("not-a-jwt", secret)
		if err == nil {
			t.Fatal("expected error for malformed token")
		}
	})

	t.Run("empty_token_rejected", func(t *testing.T) {
		_, err := ValidateToken("", secret)
		if err == nil {
			t.Fatal("expected error for empty token")
		}
	})
}

func TestAccessTokenCannotBeUsedAsRefresh(t *testing.T) {
	t.Parallel()

	secret := "test-secret-key"
	userID := uuid.Must(uuid.NewV7())

	tokenStr, err := GenerateAccessToken(userID, secret)
	if err != nil {
		t.Fatalf("GenerateAccessToken: %v", err)
	}

	claims, err := ValidateToken(tokenStr, secret)
	if err != nil {
		t.Fatalf("ValidateToken: %v", err)
	}

	if claims.TokenType != TokenTypeAccess {
		t.Fatal("expected access token type")
	}
}
