// Package i18n provides internationalization for the Senpay application.
//
// FCIS: Core — pure functions, no I/O, deterministic.
package i18n

import (
	"fmt"

	"senpay/internal/types"
)

// ResolveErrorMessage resolves a DomainError message for the given language.
//
// It looks up the error code in the locale data. For format-string errors
// (like MISSING_FIELD), it uses DomainError.Args as formatting arguments.
// Falls back to err.Message (Indonesian default) when:
//   - The error code is not found in the locale data
//   - The locale data has not been loaded
//   - The language is not supported
//   - The locale value contains format specifiers but Args are empty
func ResolveErrorMessage(err types.DomainError, lang string) string {
	key := "err_" + err.Code

	// Try to look up the key in the locale data.
	msg := T(key, lang)
	if msg == key {
		// Key not found — fall back to the default Indonesian message.
		return err.Message
	}

	// If the error has format arguments, apply them.
	if len(err.Args) > 0 {
		return fmt.Sprintf(msg, err.Args...)
	}

	// If the locale message contains format specifiers but no args are provided,
	// fall back to the pre-formatted default message (Indonesian).
	if containsFormatSpecifier(msg) {
		return err.Message
	}

	return msg
}

// containsFormatSpecifier checks if a string contains fmt.Sprintf-style
// format specifiers like %s, %d, %v, etc.
func containsFormatSpecifier(s string) bool {
	for i := 0; i < len(s)-1; i++ {
		if s[i] == '%' {
			switch s[i+1] {
			case 's', 'd', 'v', 'f', 't', 'q', 'x', 'X', 'b', 'o', 'U', 'e', 'E', 'g', 'G':
				return true
			}
		}
	}
	return false
}
