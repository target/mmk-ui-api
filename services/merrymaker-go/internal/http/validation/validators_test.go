package validation

import (
	"regexp"
	"testing"
)

const errNameRequired = "Name is required."

func TestRequired(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		maxLen    int
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid input",
			fieldName: "Name",
			maxLen:    10,
			value:     "valid",
			wantErr:   false,
		},
		{
			name:      "empty string",
			fieldName: "Name",
			maxLen:    10,
			value:     "",
			wantErr:   true,
			errMsg:    errNameRequired,
		},
		{
			name:      "whitespace only",
			fieldName: "Name",
			maxLen:    10,
			value:     "   ",
			wantErr:   true,
			errMsg:    errNameRequired,
		},
		{
			name:      "exceeds max length",
			fieldName: "Name",
			maxLen:    5,
			value:     "toolong",
			wantErr:   true,
			errMsg:    "Name cannot exceed 5 characters.",
		},
		{
			name:      "exactly max length",
			fieldName: "Name",
			maxLen:    5,
			value:     "exact",
			wantErr:   false,
		},
		{
			name:      "unicode characters within limit",
			fieldName: "Name",
			maxLen:    5,
			value:     "🚀🚀🚀🚀🚀", // 5 emoji characters (each is multiple bytes)
			wantErr:   false,
		},
		{
			name:      "unicode characters exceeds limit",
			fieldName: "Name",
			maxLen:    5,
			value:     "🚀🚀🚀🚀🚀🚀", // 6 emoji characters
			wantErr:   true,
			errMsg:    "Name cannot exceed 5 characters.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Required(tt.fieldName, tt.maxLen)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("Required() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("Required() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("Required() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestRequiredRange(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		min       int
		max       int
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid input",
			fieldName: "Name",
			min:       3,
			max:       10,
			value:     "valid",
			wantErr:   false,
		},
		{
			name:      "empty string",
			fieldName: "Name",
			min:       3,
			max:       10,
			value:     "",
			wantErr:   true,
			errMsg:    errNameRequired,
		},
		{
			name:      "too short",
			fieldName: "Name",
			min:       5,
			max:       10,
			value:     "ab",
			wantErr:   true,
			errMsg:    "Name must be between 5 and 10 characters.",
		},
		{
			name:      "too long",
			fieldName: "Name",
			min:       3,
			max:       5,
			value:     "toolong",
			wantErr:   true,
			errMsg:    "Name must be between 3 and 5 characters.",
		},
		{
			name:      "exactly min length",
			fieldName: "Name",
			min:       3,
			max:       10,
			value:     "abc",
			wantErr:   false,
		},
		{
			name:      "exactly max length",
			fieldName: "Name",
			min:       3,
			max:       5,
			value:     "abcde",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := RequiredRange(tt.fieldName, tt.min, tt.max)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("RequiredRange() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("RequiredRange() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("RequiredRange() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestIntRange(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		min       int
		max       int
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid integer",
			fieldName: "Count",
			min:       1,
			max:       100,
			value:     "50",
			wantErr:   false,
		},
		{
			name:      "below minimum",
			fieldName: "Count",
			min:       10,
			max:       100,
			value:     "5",
			wantErr:   true,
			errMsg:    "Count must be between 10 and 100.",
		},
		{
			name:      "above maximum",
			fieldName: "Count",
			min:       1,
			max:       10,
			value:     "20",
			wantErr:   true,
			errMsg:    "Count must be between 1 and 10.",
		},
		{
			name:      "not a number",
			fieldName: "Count",
			min:       1,
			max:       100,
			value:     "abc",
			wantErr:   true,
			errMsg:    "Count must be a number.",
		},
		{
			name:      "empty string",
			fieldName: "Count",
			min:       1,
			max:       100,
			value:     "",
			wantErr:   true,
			errMsg:    "Count must be a number.",
		},
		{
			name:      "exactly minimum",
			fieldName: "Count",
			min:       10,
			max:       100,
			value:     "10",
			wantErr:   false,
		},
		{
			name:      "exactly maximum",
			fieldName: "Count",
			min:       1,
			max:       100,
			value:     "100",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := IntRange(tt.fieldName, tt.min, tt.max)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("IntRange() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("IntRange() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("IntRange() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestHTTPSURL(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		maxLen    int
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid HTTPS URL",
			fieldName: "URI",
			maxLen:    100,
			value:     "https://example.com",
			wantErr:   false,
		},
		{
			name:      "valid HTTP URL",
			fieldName: "URI",
			maxLen:    100,
			value:     "http://example.com",
			wantErr:   false,
		},
		{
			name:      "empty string",
			fieldName: "URI",
			maxLen:    100,
			value:     "",
			wantErr:   true,
			errMsg:    "URI is required.",
		},
		{
			name:      "exceeds max length",
			fieldName: "URI",
			maxLen:    10,
			value:     "https://example.com/very/long/path",
			wantErr:   true,
			errMsg:    "URI cannot exceed 10 characters.",
		},
		{
			name:      "invalid URL",
			fieldName: "URI",
			maxLen:    100,
			value:     "not a url",
			wantErr:   true,
			errMsg:    "Enter a valid http(s) URL.",
		},
		{
			name:      "missing scheme",
			fieldName: "URI",
			maxLen:    100,
			value:     "example.com",
			wantErr:   true,
			errMsg:    "Enter a valid http(s) URL.",
		},
		{
			name:      "invalid scheme",
			fieldName: "URI",
			maxLen:    100,
			value:     "ftp://example.com",
			wantErr:   true,
			errMsg:    "Enter a valid http(s) URL.",
		},
		{
			name:      "missing host",
			fieldName: "URI",
			maxLen:    100,
			value:     "https://",
			wantErr:   true,
			errMsg:    "Enter a valid http(s) URL.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := HTTPSURL(tt.fieldName, tt.maxLen)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("HTTPSURL() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("HTTPSURL() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("HTTPSURL() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestOneOf(t *testing.T) {
	tests := []struct {
		name      string
		fieldName string
		options   []string
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "valid option exact case",
			fieldName: "Type",
			options:   []string{"GET", "POST", "PUT"},
			value:     "GET",
			wantErr:   false,
		},
		{
			name:      "valid option different case",
			fieldName: "Type",
			options:   []string{"GET", "POST", "PUT"},
			value:     "get",
			wantErr:   false,
		},
		{
			name:      "invalid option",
			fieldName: "Type",
			options:   []string{"GET", "POST", "PUT"},
			value:     "DELETE",
			wantErr:   true,
			errMsg:    "Type must be one of: GET, POST, PUT",
		},
		{
			name:      "empty string",
			fieldName: "Type",
			options:   []string{"GET", "POST", "PUT"},
			value:     "",
			wantErr:   true,
			errMsg:    "Type must be one of: GET, POST, PUT",
		},
		{
			name:      "whitespace trimmed",
			fieldName: "Type",
			options:   []string{"GET", "POST", "PUT"},
			value:     "  POST  ",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := OneOf(tt.fieldName, tt.options)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("OneOf() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("OneOf() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("OneOf() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestPattern(t *testing.T) {
	alphanumericRe := regexp.MustCompile(`^[A-Za-z0-9]+$`)
	secretNameRe := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

	tests := []struct {
		name      string
		fieldName string
		re        *regexp.Regexp
		value     string
		wantErr   bool
		errMsg    string
	}{
		{
			name:      "matches pattern",
			fieldName: "Name",
			re:        alphanumericRe,
			value:     "abc123",
			wantErr:   false,
		},
		{
			name:      "does not match pattern",
			fieldName: "Name",
			re:        alphanumericRe,
			value:     "abc-123",
			wantErr:   true,
			errMsg:    "Name has an invalid format.",
		},
		{
			name:      "empty string allowed",
			fieldName: "Name",
			re:        alphanumericRe,
			value:     "",
			wantErr:   false,
		},
		{
			name:      "secret name valid",
			fieldName: "SecretName",
			re:        secretNameRe,
			value:     "my_secret-123",
			wantErr:   false,
		},
		{
			name:      "secret name invalid start",
			fieldName: "SecretName",
			re:        secretNameRe,
			value:     "-invalid",
			wantErr:   true,
			errMsg:    "SecretName has an invalid format.",
		},
		{
			name:      "whitespace trimmed before validation",
			fieldName: "Name",
			re:        alphanumericRe,
			value:     "  abc123  ",
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := Pattern(tt.fieldName, tt.re)
			err := v(tt.value)
			if tt.wantErr && err == "" {
				t.Errorf("Pattern() expected error but got none")
			}
			if !tt.wantErr && err != "" {
				t.Errorf("Pattern() unexpected error: %v", err)
			}
			if tt.wantErr && err != tt.errMsg {
				t.Errorf("Pattern() error = %v, want %v", err, tt.errMsg)
			}
		})
	}
}

func TestFieldValidator_SingleField(t *testing.T) {
	fv := New().Validate("name", "test", Required("Name", 10))
	errs := fv.Errors()
	if len(errs) != 0 {
		t.Errorf("Expected no errors, got %v", errs)
	}
}

func TestFieldValidator_SingleFieldWithError(t *testing.T) {
	fv := New().Validate("name", "", Required("Name", 10))
	errs := fv.Errors()
	if len(errs) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errs))
	}
	if errs["name"] != errNameRequired {
		t.Errorf("Expected %q, got %v", errNameRequired, errs["name"])
	}
}

func TestFieldValidator_MultipleFields(t *testing.T) {
	fv := New().
		Validate("name", "test", Required("Name", 10)).
		Validate("count", "5", IntRange("Count", 1, 10))
	errs := fv.Errors()
	if len(errs) != 0 {
		t.Errorf("Expected no errors, got %v", errs)
	}
}

func TestFieldValidator_MultipleFieldsWithErrors(t *testing.T) {
	fv := New().
		Validate("name", "", Required("Name", 10)).
		Validate("count", "100", IntRange("Count", 1, 10))
	errs := fv.Errors()
	if len(errs) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(errs))
	}
	if errs["name"] != errNameRequired {
		t.Errorf("Expected %q, got %v", errNameRequired, errs["name"])
	}
	if errs["count"] != "Count must be between 1 and 10." {
		t.Errorf("Expected 'Count must be between 1 and 10.', got %v", errs["count"])
	}
}

func TestFieldValidator_StopsAtFirstError(t *testing.T) {
	fv := New().Validate("name", "", Required("Name", 10), Pattern("Name", regexp.MustCompile(`^[A-Z]+$`)))
	errs := fv.Errors()
	if len(errs) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errs))
	}
	// Should stop at Required error, not reach Pattern
	if errs["name"] != errNameRequired {
		t.Errorf("Expected %q, got %v", errNameRequired, errs["name"])
	}
}

func TestFieldValidator_SecondValidatorTriggers(t *testing.T) {
	fv := New().Validate("name", "abc", Required("Name", 10), Pattern("Name", regexp.MustCompile(`^[A-Z]+$`)))
	errs := fv.Errors()
	if len(errs) != 1 {
		t.Errorf("Expected 1 error, got %d", len(errs))
	}
	// Should pass Required, fail Pattern
	if errs["name"] != "Name has an invalid format." {
		t.Errorf("Expected 'Name has an invalid format.', got %v", errs["name"])
	}
}

func TestFieldValidator_EmptyErrors(t *testing.T) {
	fv := New()
	errs := fv.Errors()
	if len(errs) != 0 {
		t.Errorf("Expected empty errors map, got %v", errs)
	}
}
