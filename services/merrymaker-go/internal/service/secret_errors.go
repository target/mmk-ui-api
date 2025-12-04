package service

import (
	"strings"
	"unicode/utf8"
)

const (
	// providerScriptErrorPrefix is the prefix used for user-facing provider script errors.
	providerScriptErrorPrefix = "Refresh script failed during validation"
	// providerScriptErrorMaxLen caps the length of the user-facing message to avoid giant banners.
	providerScriptErrorMaxLen = 500
)

// SecretProviderScriptError is returned when validating a dynamic secret's provider script fails.
// It carries a sanitized, user-facing message while preserving the original error for logging or inspection.
type SecretProviderScriptError struct {
	scriptPath  string
	userMessage string
	err         error
}

// Error implements the error interface.
func (e *SecretProviderScriptError) Error() string {
	if e == nil {
		return ""
	}
	return e.userMessage
}

// Unwrap exposes the underlying error for errors.Is / errors.As checks.
func (e *SecretProviderScriptError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.err
}

// UserMessage returns the sanitized, user-facing error message.
func (e *SecretProviderScriptError) UserMessage() string {
	if e == nil {
		return ""
	}
	return e.userMessage
}

// NewSecretProviderScriptError constructs a SecretProviderScriptError with a sanitized message.
func NewSecretProviderScriptError(scriptPath string, err error) *SecretProviderScriptError {
	return &SecretProviderScriptError{
		scriptPath:  scriptPath,
		userMessage: buildProviderScriptUserMessage(scriptPath, err),
		err:         err,
	}
}

func buildProviderScriptUserMessage(scriptPath string, err error) string {
	var b strings.Builder
	b.WriteString(providerScriptErrorPrefix)

	if scriptPath = strings.TrimSpace(scriptPath); scriptPath != "" {
		b.WriteString(" (")
		b.WriteString(scriptPath)
		b.WriteString(")")
	}

	rawMessage := strings.TrimSpace(errorMessage(err))
	if rawMessage != "" {
		// Replace line breaks and compress whitespace to keep banner tidy.
		normalized := strings.Join(strings.Fields(strings.ReplaceAll(rawMessage, "\n", " ")), " ")
		// Truncate if needed to avoid overwhelming the UI.
		if utf8.RuneCountInString(normalized) > providerScriptErrorMaxLen {
			normalized = truncate(normalized, providerScriptErrorMaxLen)
		}
		b.WriteString(": ")
		b.WriteString(normalized)
	} else {
		b.WriteString(".")
	}

	return b.String()
}

func truncate(s string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	trimmed := strings.Builder{}
	count := 0
	for _, r := range s {
		if count >= limit {
			break
		}
		trimmed.WriteRune(r)
		count++
	}
	trimmed.WriteString("…")
	return trimmed.String()
}

func errorMessage(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
