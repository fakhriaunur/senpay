package types

import "context"

// contextKey is used for storing values in request context.
type contextKey string

const (
	// CtxKeyAcceptLanguage is the context key for the Accept-Language header value.
	CtxKeyAcceptLanguage contextKey = "accept_language"
)

// GetAcceptLanguage extracts the Accept-Language value from the request context.
// Returns empty string if not set.
func GetAcceptLanguage(ctx context.Context) string {
	if lang, ok := ctx.Value(CtxKeyAcceptLanguage).(string); ok {
		return lang
	}
	return ""
}

// WithAcceptLanguage returns a new context with the Accept-Language value set.
func WithAcceptLanguage(ctx context.Context, lang string) context.Context {
	return context.WithValue(ctx, CtxKeyAcceptLanguage, lang)
}
