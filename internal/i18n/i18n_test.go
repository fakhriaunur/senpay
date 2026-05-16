package i18n

import (
	"testing"

	"senpay/internal/types"
)

func TestT_ReturnsTranslation(t *testing.T) {
	// Load test locales
	err := LoadLocale("id", []byte(`{"hello": "Halo", "greeting": "Halo, %s!"}`))
	if err != nil {
		t.Fatalf("LoadLocale: %v", err)
	}
	err = LoadLocale("en", []byte(`{"hello": "Hello", "greeting": "Hello, %s!"}`))
	if err != nil {
		t.Fatalf("LoadLocale: %v", err)
	}

	tests := []struct {
		name     string
		key      string
		lang     string
		args     []interface{}
		expected string
	}{
		{
			name:     "indonesian translation",
			key:      "hello",
			lang:     "id",
			expected: "Halo",
		},
		{
			name:     "english translation",
			key:      "hello",
			lang:     "en",
			expected: "Hello",
		},
		{
			name:     "with format arg",
			key:      "greeting",
			lang:     "id",
			args:     []interface{}{"World"},
			expected: "Halo, World!",
		},
		{
			name:     "missing key returns key",
			key:      "nonexistent",
			lang:     "id",
			expected: "nonexistent",
		},
		{
			name:     "empty lang defaults to id",
			key:      "hello",
			lang:     "",
			expected: "Halo",
		},
		{
			name:     "unknown lang falls back to id",
			key:      "hello",
			lang:     "fr",
			expected: "Halo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := T(tt.key, tt.lang, tt.args...)
			if got != tt.expected {
				t.Errorf("T(%q, %q) = %q, want %q", tt.key, tt.lang, got, tt.expected)
			}
		})
	}
}

func TestT_MissingKeyReturnsKey(t *testing.T) {
	err := LoadLocale("id", []byte(`{"existing": "ada"}`))
	if err != nil {
		t.Fatalf("LoadLocale: %v", err)
	}

	got := T("nonexistent", "id")
	if got != "nonexistent" {
		t.Errorf("T(missing key) = %q, want %q", got, "nonexistent")
	}
}

func TestLoadLocale_InvalidJSON(t *testing.T) {
	err := LoadLocale("xx", []byte(`{invalid json}`))
	if err == nil {
		t.Error("LoadLocale with invalid JSON should return error")
	}
}

func TestT_DefaultLangConstant(t *testing.T) {
	if DefaultLang != "id" {
		t.Errorf("DefaultLang = %q, want %q", DefaultLang, "id")
	}
}

func TestResolveErrorMessage_StaticError(t *testing.T) {
	_ = LoadLocale("id", []byte(`{"err_INVALID_PIN": "PIN salah"}`))
	_ = LoadLocale("en", []byte(`{"err_INVALID_PIN": "Invalid PIN"}`))

	tests := []struct {
		name     string
		code     string
		message  string
		lang     string
		expected string
	}{
		{
			name:     "indonesian default",
			code:     "INVALID_PIN",
			message:  "PIN salah",
			lang:     "id",
			expected: "PIN salah",
		},
		{
			name:     "english translation",
			code:     "INVALID_PIN",
			message:  "PIN salah",
			lang:     "en",
			expected: "Invalid PIN",
		},
		{
			name:     "unknown lang falls back to indonesian",
			code:     "INVALID_PIN",
			message:  "PIN salah",
			lang:     "fr",
			expected: "PIN salah",
		},
		{
			name:     "missing code falls back to default message",
			code:     "UNKNOWN_CODE",
			message:  "Default message",
			lang:     "id",
			expected: "Default message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := types.DomainError{
				Code:    tt.code,
				Message: tt.message,
			}
			got := ResolveErrorMessage(err, tt.lang)
			if got != tt.expected {
				t.Errorf("ResolveErrorMessage(%q, %q) = %q, want %q", tt.code, tt.lang, got, tt.expected)
			}
		})
	}
}

func TestResolveErrorMessage_FormatString(t *testing.T) {
	_ = LoadLocale("id", []byte(`{"err_MISSING_FIELD": "Field %s wajib diisi"}`))
	_ = LoadLocale("en", []byte(`{"err_MISSING_FIELD": "%s is required"}`))

	tests := []struct {
		name     string
		code     string
		message  string
		args     []interface{}
		lang     string
		expected string
	}{
		{
			name:     "indonesian with arg",
			code:     "MISSING_FIELD",
			message:  "Field pin wajib diisi",
			args:     []interface{}{"pin"},
			lang:     "id",
			expected: "Field pin wajib diisi",
		},
		{
			name:     "english with arg",
			code:     "MISSING_FIELD",
			message:  "Field pin wajib diisi",
			args:     []interface{}{"pin"},
			lang:     "en",
			expected: "pin is required",
		},
		{
			name:     "no args falls back to default message",
			code:     "MISSING_FIELD",
			message:  "Field pin wajib diisi",
			lang:     "id",
			expected: "Field pin wajib diisi",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := types.DomainError{
				Code:    tt.code,
				Message: tt.message,
				Args:    tt.args,
			}
			got := ResolveErrorMessage(err, tt.lang)
			if got != tt.expected {
				t.Errorf("ResolveErrorMessage(%q, %q) = %q, want %q", tt.code, tt.lang, got, tt.expected)
			}
		})
	}
}
