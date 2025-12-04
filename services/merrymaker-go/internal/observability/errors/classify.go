package errors

import (
	goerrors "errors"
	"reflect"
	"strings"
)

// Classify returns a normalized error type name suitable for tagging metrics/logs.
// It unwraps errors until the innermost concrete type is found and converts it to snake_case-ish.
func Classify(err error) string {
	if err == nil {
		return ""
	}

	// Unwrap to the innermost error for better signal.
	for {
		unwrapped := goerrors.Unwrap(err)
		if unwrapped == nil {
			break
		}
		err = unwrapped
	}

	t := reflect.TypeOf(err)
	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t == nil {
		return "unknown"
	}

	name := strings.ToLower(strings.ReplaceAll(t.String(), "*", ""))
	name = strings.ReplaceAll(name, ".", "_")
	if name == "" {
		return "unknown"
	}
	return name
}
