// Package i18n provides internationalization for the Senpay TUI.
//
// FCIS: Core — pure functions, no I/O, deterministic.
// Locale data is loaded at startup and cached in memory.
package i18n

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

// DefaultLang is the default language.
const DefaultLang = "id"

// localesCache caches loaded locale data by language code.
var (
	localesCache map[string]map[string]string
	loadOnce     sync.Once
	loadMu       sync.RWMutex
)

// T returns the translated string for the given key and language.
// Returns the key itself if the translation is not found.
// Falls back to Indonesian if the language is not supported.
// Supports fmt.Sprintf-style formatting via variadic args.
func T(key string, lang string, args ...interface{}) string {
	loadOnce.Do(loadLocales)

	// Normalize language: if empty or unknown, fall back to default.
	if lang == "" {
		lang = DefaultLang
	}

	loadMu.RLock()
	data, ok := localesCache[lang]
	loadMu.RUnlock()

	if !ok {
		// Fall back to default language
		loadMu.RLock()
		data = localesCache[DefaultLang]
		loadMu.RUnlock()
	}

	if data == nil {
		return key
	}

	val, ok := data[key]
	if !ok {
		return key
	}

	if len(args) > 0 {
		return fmt.Sprintf(val, args...)
	}

	return val
}

// LoadLocale manually loads a locale from a JSON byte slice.
// Used for testing.
func LoadLocale(lang string, data []byte) error {
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("i18n: parse %s locale: %w", lang, err)
	}

	loadMu.Lock()
	defer loadMu.Unlock()

	if localesCache == nil {
		localesCache = make(map[string]map[string]string)
	}
	localesCache[lang] = m
	return nil
}

// loadLocales loads all locale files from the locales/ directory.
func loadLocales() {
	loadMu.Lock()
	defer loadMu.Unlock()

	if localesCache != nil {
		return
	}

	localesCache = make(map[string]map[string]string)

	// Load supported locales.
	for _, lang := range []string{"id", "en"} {
		data, err := os.ReadFile("locales/" + lang + ".json")
		if err != nil {
			// In test environments or non-standard CWD, try alternative paths.
			data, err = os.ReadFile("../../locales/" + lang + ".json")
			if err != nil {
				// Silently skip — tests can load manually.
				continue
			}
		}

		var m map[string]string
		if err := json.Unmarshal(data, &m); err != nil {
			continue
		}
		localesCache[lang] = m
	}
}
