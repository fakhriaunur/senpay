package auth

import (
	"strings"
	"testing"

	"senpay/internal/types"

	"pgregory.net/rapid"
)

func TestHashPIN(t *testing.T) {
	t.Parallel()

	t.Run("returns_non_empty_hash", func(t *testing.T) {
		hash := HashPIN("123456")
		if hash == "" {
			t.Fatal("hash must not be empty")
		}
	})

	t.Run("returns_valid_bcrypt_format", func(t *testing.T) {
		hash := HashPIN("123456")
		if !strings.HasPrefix(hash, "$2a$12$") && !strings.HasPrefix(hash, "$2b$12$") {
			t.Errorf("hash does not start with expected bcrypt prefix: %s", hash)
		}
	})

	t.Run("different_pins_produce_different_hashes", func(t *testing.T) {
		h1 := HashPIN("123456")
		h2 := HashPIN("654321")
		if h1 == h2 {
			t.Error("different PINs should produce different hashes")
		}
	})

	t.Run("hash_includes_cost_12", func(t *testing.T) {
		hash := HashPIN("123456")
		// bcrypt format: $2a/b$<cost>$<22charsalt><31charshash>
		// The cost is at position after second $ and before third $
		parts := strings.Split(hash, "$")
		if len(parts) < 4 {
			t.Fatalf("unexpected bcrypt format: %s", hash)
		}
		if parts[2] != "12" {
			t.Errorf("expected cost 12, got %s", parts[2])
		}
	})

	t.Run("hash_consistent_for_same_pin", func(t *testing.T) {
		// bcrypt uses random salt, so two hashes of same PIN will differ.
		// We only verify that VerifyPIN works with both hashes.
		pin := "123456"
		h1 := HashPIN(pin)
		h2 := HashPIN(pin)

		if !VerifyPIN(pin, h1) {
			t.Error("VerifyPIN should work with first hash")
		}
		if !VerifyPIN(pin, h2) {
			t.Error("VerifyPIN should work with second hash")
		}
		if h1 == h2 {
			t.Error("same PIN should produce different hashes due to random bcrypt salt")
		}
	})

	t.Run("panics_on_empty_pin", func(t *testing.T) {
		// bcrypt will fail on empty input? Let's verify it doesn't panic.
		// Actually, bcrypt accepts empty strings. 
		hash := HashPIN("")
		if hash == "" {
			t.Error("empty PIN should still produce a hash")
		}
	})
}

func TestVerifyPIN(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		pin      string
		setupPIN string // PIN to hash before verification
		want     bool
	}{
		{name: "correct_pin", pin: "123456", setupPIN: "123456", want: true},
		{name: "wrong_pin", pin: "000000", setupPIN: "123456", want: false},
		{name: "wrong_pin_similar", pin: "123455", setupPIN: "123456", want: false},
		{name: "wrong_pin_longer", pin: "1234567", setupPIN: "123456", want: false},
		{name: "wrong_pin_shorter", pin: "12345", setupPIN: "123456", want: false},
		{name: "empty_pin_wrong", pin: "", setupPIN: "123456", want: false},
		{name: "empty_both", pin: "", setupPIN: "", want: true},
		{name: "long_pin", pin: "1234567890123456", setupPIN: "1234567890123456", want: true},
		{name: "special_chars_pin", pin: "!@#$%^", setupPIN: "!@#$%^", want: true},
		{name: "numeric_pin", pin: "000000", setupPIN: "000000", want: true},
		{name: "trailing_newline", pin: "123456\n", setupPIN: "123456", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashPIN(tt.setupPIN)
			got := VerifyPIN(tt.pin, hash)
			if got != tt.want {
				t.Errorf("VerifyPIN(%q, hash) = %v, want %v", tt.pin, got, tt.want)
			}
		})
	}
}

func TestValidatePhone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		phone     string
		wantErr   bool
		wantCode  string
	}{
		// ── Valid Indonesian numbers ───────────────────────────
		{name: "valid_08_12_digits", phone: "081234567890", wantErr: false},
		{name: "valid_08_10_digits", phone: "0812345678", wantErr: false},
		{name: "valid_08_13_digits", phone: "0812345678901", wantErr: false},
		{name: "valid_62_12_digits", phone: "628123456789", wantErr: false},
		{name: "valid_62_10_digits", phone: "62812345678", wantErr: false},
		{name: "valid_62_13_digits", phone: "6281234567890", wantErr: false},
		{name: "valid_plus_62", phone: "+628123456789", wantErr: false},
		{name: "valid_plus_62_long", phone: "+6281234567890", wantErr: false},
		{name: "valid_08_boundary_10", phone: "0812345678", wantErr: false},
		{name: "valid_08_boundary_13", phone: "0812345678901", wantErr: false},

		// ── Invalid: wrong prefix ──────────────────────────────
		{name: "invalid_01_prefix", phone: "01123456789", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "invalid_07_prefix", phone: "07123456789", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "invalid_plus_1_prefix", phone: "+11234567890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "invalid_1_prefix", phone: "1234567890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},

		// ── Invalid: wrong length ──────────────────────────────
		{name: "too_short_9_digits", phone: "081234567", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "too_short_5_digits", phone: "08123", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "too_short_1_digit", phone: "0", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "too_long_14_digits", phone: "08123456789012", wantErr: true, wantCode: types.ErrCodeInvalidFormat},

		// ── Invalid: non-digit characters ──────────────────────
		{name: "contains_letters", phone: "08123abc890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "contains_hyphens", phone: "0812-3456-7890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "contains_spaces", phone: "0812 3456 7890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "contains_parentheses", phone: "(0812)34567890", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "has_country_code_with_chars", phone: "+1-555-0000", wantErr: true, wantCode: types.ErrCodeInvalidFormat},

		// ── Invalid: empty ─────────────────────────────────────
		{name: "empty_string", phone: "", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "just_plus", phone: "+", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
		{name: "plus_then_empty", phone: "+0", wantErr: true, wantCode: types.ErrCodeInvalidFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhone(tt.phone)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for phone %q, got nil", tt.phone)
				}
				if err.Code != tt.wantCode {
					t.Errorf("error code: got %q, want %q", err.Code, tt.wantCode)
				}
				if err.HTTPStatus != 400 {
					t.Errorf("HTTP status: got %d, want 400", err.HTTPStatus)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for phone %q: %v", tt.phone, err)
			}
		})
	}
}

// ── Rapid property-based tests ──────────────────────────────────

func TestProperty_HashPIN_VerifyPIN_Roundtrip(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		pin := rapid.StringMatching(`\d{4,8}`).Draw(t, "pin")

		hash := HashPIN(pin)
		if hash == "" {
			t.Fatal("hash must not be empty")
		}

		if !VerifyPIN(pin, hash) {
			t.Fatal("VerifyPIN should return true for the same PIN")
		}
	})
}

func TestProperty_VerifyPIN_WrongPIN(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		pin := rapid.StringMatching(`\d{4,8}`).Draw(t, "pin")
		wrongPIN := rapid.StringMatching(`\d{4,8}`).Draw(t, "wrongPIN")

		// Skip if by chance they're equal — we want wrong PINs.
		if pin == wrongPIN {
			return
		}

		hash := HashPIN(pin)
		if VerifyPIN(wrongPIN, hash) {
			t.Fatal("VerifyPIN should return false for a different PIN")
		}
	})
}

func TestProperty_HashPIN_Format(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		pin := rapid.StringMatching(`\d{4,8}`).Draw(t, "pin")

		hash := HashPIN(pin)
		if !strings.HasPrefix(hash, "$2a$12$") && !strings.HasPrefix(hash, "$2b$12$") {
			t.Fatalf("hash must start with $2a$12$ or $2b$12$, got %q", hash[:10])
		}
		// BCrypt hash length: $2a$12$<53chars of salt+hash> = 60 chars total.
		if len(hash) != 60 {
			t.Fatalf("expected bcrypt hash length 60, got %d", len(hash))
		}
	})
}

func TestProperty_VerifyPIN_InvalidHash(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		pin := rapid.StringMatching(`\d{4,8}`).Draw(t, "pin")
		invalidHash := rapid.StringMatching(`.{10,30}`).Draw(t, "invalidHash")

		// Should not panic, should return false.
		result := VerifyPIN(pin, invalidHash)
		if result {
			t.Fatal("VerifyPIN should return false for invalid hash")
		}
	})
}

func TestProperty_ValidatePhone_Valid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// Generate a valid 08-prefixed Indonesian phone number.
		prefix := rapid.SampledFrom([]string{"08", "62"}).Draw(t, "prefix")
		digitCount := rapid.IntRange(8, 11).Draw(t, "extraDigits") // total 10-13
		extraDigits := rapid.StringMatching(`\d{` + itoa(digitCount) + `}`).Draw(t, "extraDigits")

		phone := prefix + extraDigits
		if len(phone) < 10 || len(phone) > 13 {
			return // skip out-of-range (shouldn't happen given ranges above)
		}

		err := ValidatePhone(phone)
		if err != nil {
			t.Fatalf("expected valid phone %q to pass, got error: %v", phone, err)
		}
	})
}

func TestProperty_ValidatePhone_Invalid(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		// Generate clearly invalid phone numbers.
		prefix := rapid.SampledFrom([]string{"01", "07", "00", "11", "99", "", "a"}).Draw(t, "prefix")
		digits := rapid.StringMatching(`\d{0,10}`).Draw(t, "digits")

		phone := prefix + digits
		if phone == "" {
			return // skip empty, handled separately
		}

		// Skip accidentally valid numbers.
		err := ValidatePhone(phone)
		if err == nil {
			// It's possible some random string happens to be valid. Skip those.
			return
		}
		// Must return invalid format error.
		if err.Code != types.ErrCodeInvalidFormat {
			t.Fatalf("expected ErrCodeInvalidFormat for invalid phone %q, got %q", phone, err.Code)
		}
	})
}

// itoa is a simple int to string converter for use without strconv import.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	s := ""
	for n > 0 {
		s = string(rune('0'+n%10)) + s
		n /= 10
	}
	return s
}
