package i18n

import (
	"testing"
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
