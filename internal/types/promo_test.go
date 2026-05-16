package types

import (
	"testing"
)

func TestParsePromoCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    PromoCode
		wantErr bool
	}{
		// Valid codes
		{name: "simple_alphanumeric", input: "BEBASFEE", want: PromoCode("BEBASFEE")},
		{name: "with_hyphen", input: "GRATIS-ONGKIR", want: PromoCode("GRATIS-ONGKIR")},
		{name: "mixed_case", input: "Promo-2026", want: PromoCode("Promo-2026")},
		{name: "all_lowercase", input: "bebasfee", want: PromoCode("bebasfee")},
		{name: "all_numbers", input: "123456", want: PromoCode("123456")},
		{name: "numbers_and_letters", input: "PROMO2026", want: PromoCode("PROMO2026")},
		{name: "multi_hyphen", input: "SUPER-PROMO-2026", want: PromoCode("SUPER-PROMO-2026")},
		{name: "single_char", input: "X", want: PromoCode("X")},
		{name: "single_digit", input: "0", want: PromoCode("0")},

		// Invalid codes
		{name: "empty_string", input: "", wantErr: true},
		{name: "with_space", input: "BEBAS FEE", wantErr: true},
		{name: "with_underscore", input: "BEBAS_FEE", wantErr: true},
		{name: "special_chars", input: "BEBAS!!", wantErr: true},
		{name: "with_dollar", input: "PROMO$", wantErr: true},
		{name: "with_at", input: "PROMO@2026", wantErr: true},
		{name: "with_hash", input: "PROMO#2026", wantErr: true},
		{name: "with_dot", input: "PROMO.2026", wantErr: true},
		{name: "with_plus", input: "PROMO+2026", wantErr: true},
		{name: "unicode_chars", input: "PRO-MÔ", wantErr: true},
		{name: "with_parentheses", input: "PROMO(1)", wantErr: true},
		{name: "leading_space", input: " PROMO", wantErr: true},
		{name: "trailing_space", input: "PROMO ", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePromoCode(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("ParsePromoCode(%q) expected error, got nil", tt.input)
				}
				// Verify it returns a DomainError.
				if _, ok := err.(DomainError); !ok {
					t.Fatalf("ParsePromoCode(%q) error should be DomainError, got %T: %v", tt.input, err, err)
				}
				// Verify error code.
				domainErr := err.(DomainError)
				if domainErr.Code != ErrCodeInvalidFormat {
					t.Errorf("ParsePromoCode(%q) error code = %q, want %q", tt.input, domainErr.Code, ErrCodeInvalidFormat)
				}
				if got != "" {
					t.Errorf("ParsePromoCode(%q) = %q on error, want empty", tt.input, got)
				}
				return
			}

			if err != nil {
				t.Fatalf("ParsePromoCode(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("ParsePromoCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestPromoCodeString(t *testing.T) {
	t.Parallel()

	pc := PromoCode("BEBASFEE")
	if pc.String() != "BEBASFEE" {
		t.Errorf("PromoCode.String() = %q, want %q", pc.String(), "BEBASFEE")
	}

	// Test that PromoCode can be used as a string directly.
	_ = string(pc) // should compile
}
