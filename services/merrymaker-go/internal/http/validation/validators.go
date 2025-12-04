package validation

import (
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Validator is a function that validates a string value and returns an error message if invalid.
type Validator func(v string) string

// Required validates that a field is not empty and does not exceed maxLen characters.
// Uses rune count for proper Unicode support.
func Required(fieldName string, maxLen int) Validator {
	return func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return fieldName + " is required."
		}
		if utf8.RuneCountInString(v) > maxLen {
			return fmt.Sprintf("%s cannot exceed %d characters.", fieldName, maxLen)
		}
		return ""
	}
}

// RequiredRange validates that a field is not empty and is between minLen and maxLen characters.
// Uses rune count for proper Unicode support.
func RequiredRange(fieldName string, minLen, maxLen int) Validator {
	return func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return fieldName + " is required."
		}
		n := utf8.RuneCountInString(v)
		if n < minLen || n > maxLen {
			return fmt.Sprintf("%s must be between %d and %d characters.", fieldName, minLen, maxLen)
		}
		return ""
	}
}

// IntRange validates that a field is a valid integer between minVal and maxVal.
func IntRange(fieldName string, minVal, maxVal int) Validator {
	return func(v string) string {
		i, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return fieldName + " must be a number."
		}
		if i < minVal || i > maxVal {
			return fmt.Sprintf("%s must be between %d and %d.", fieldName, minVal, maxVal)
		}
		return ""
	}
}

// HTTPSURL validates that a field is a valid HTTP(S) URL and does not exceed maxLen characters.
// Uses rune count for proper Unicode support.
func HTTPSURL(fieldName string, maxLen int) Validator {
	return func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return fieldName + " is required."
		}
		if utf8.RuneCountInString(v) > maxLen {
			return fmt.Sprintf("%s cannot exceed %d characters.", fieldName, maxLen)
		}
		p, e := url.Parse(v)
		if e != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return "Enter a valid http(s) URL."
		}
		return ""
	}
}

// OneOf validates that a field matches one of the provided options (case-insensitive).
func OneOf(fieldName string, options []string) Validator {
	return func(v string) string {
		v = strings.ToUpper(strings.TrimSpace(v))
		for _, opt := range options {
			if v == strings.ToUpper(opt) {
				return ""
			}
		}
		return fmt.Sprintf("%s must be one of: %s", fieldName, strings.Join(options, ", "))
	}
}

// Pattern validates that a field matches the provided regular expression.
func Pattern(fieldName string, re *regexp.Regexp) Validator {
	return func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if !re.MatchString(v) {
			return fieldName + " has an invalid format."
		}
		return ""
	}
}

// Optional validates that an optional field does not exceed maxLen characters if provided.
// Uses rune count for proper Unicode support.
func Optional(fieldName string, maxLen int) Validator {
	return func(v string) string {
		v = strings.TrimSpace(v)
		if v == "" {
			return ""
		}
		if utf8.RuneCountInString(v) > maxLen {
			return fmt.Sprintf("%s cannot exceed %d characters.", fieldName, maxLen)
		}
		return ""
	}
}

// FieldValidator provides a fluent API for validating multiple fields.
type FieldValidator struct {
	errors map[string]string
}

// New creates a new FieldValidator instance.
func New() *FieldValidator {
	return &FieldValidator{errors: make(map[string]string)}
}

// Validate validates a field with one or more validators.
// It stops at the first error for each field.
func (fv *FieldValidator) Validate(field, value string, validators ...Validator) *FieldValidator {
	for _, v := range validators {
		if err := v(value); err != "" {
			fv.errors[field] = err
			break // Stop at first error per field
		}
	}
	return fv
}

// Errors returns the accumulated validation errors.
func (fv *FieldValidator) Errors() map[string]string {
	return fv.errors
}
