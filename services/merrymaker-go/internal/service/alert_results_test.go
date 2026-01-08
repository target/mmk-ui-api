package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretRedactor_RedactString(t *testing.T) {
	tests := []struct {
		name     string
		secrets  map[string]string
		input    string
		expected string
	}{
		{
			name:     "empty secrets",
			secrets:  map[string]string{},
			input:    "some text with secret123",
			expected: "some text with secret123",
		},
		{
			name:     "basic redaction",
			secrets:  map[string]string{"__API_KEY__": "secret123"},
			input:    "Authorization: Bearer secret123",
			expected: "Authorization: Bearer __API_KEY__",
		},
		{
			name:     "url encoded secret",
			secrets:  map[string]string{"__TOKEN__": "my token"},
			input:    "https://api.example.com?token=my+token",
			expected: "https://api.example.com?token=__TOKEN__",
		},
		{
			name:     "multiple secrets",
			secrets:  map[string]string{"__KEY1__": "abc", "__KEY2__": "xyz"},
			input:    "key1=abc&key2=xyz",
			expected: "key1=__KEY1__&key2=__KEY2__",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redactor := NewSecretRedactor(tt.secrets)
			result := redactor.RedactString(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSecretRedactor_RedactHeaders(t *testing.T) {
	tests := []struct {
		name     string
		secrets  map[string]string
		headers  map[string]string
		expected map[string]string
	}{
		{
			name:     "nil headers",
			secrets:  map[string]string{},
			headers:  nil,
			expected: nil,
		},
		{
			name:     "empty headers",
			secrets:  map[string]string{},
			headers:  map[string]string{},
			expected: nil,
		},
		{
			name:    "non-sensitive headers unchanged",
			secrets: map[string]string{},
			headers: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
				"User-Agent":   "MyApp/1.0",
			},
			expected: map[string]string{
				"Content-Type": "application/json",
				"Accept":       "application/json",
				"User-Agent":   "MyApp/1.0",
			},
		},
		{
			name:    "authorization header masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"Authorization": "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
				"Content-Type":  "application/json",
			},
			expected: map[string]string{
				"Authorization": "Bearer ***",
				"Content-Type":  "application/json",
			},
		},
		{
			name:    "basic auth header masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"Authorization": "Basic dXNlcjpwYXNzd29yZA==",
			},
			expected: map[string]string{
				"Authorization": "Basic ***",
			},
		},
		{
			name:    "api-key headers masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"API-Key":   "sk_live_1234567890abcdef",
				"X-API-Key": "pk_test_abcdefghijklmnop",
				"x-api-key": "another_key_value",
			},
			expected: map[string]string{
				"API-Key":   "sk_***",
				"X-API-Key": "pk_***",
				"x-api-key": "ano***",
			},
		},
		{
			name:    "token headers masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"X-Auth-Token":   "abc123def456",
				"X-Access-Token": "ghijklmnop",
			},
			expected: map[string]string{
				"X-Auth-Token":   "abc***",
				"X-Access-Token": "ghi***",
			},
		},
		{
			name:    "cookie headers masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"Cookie":     "session_id=abc123; auth_token=xyz789",
				"Set-Cookie": "session=value; Path=/; HttpOnly",
			},
			expected: map[string]string{
				"Cookie":     "session_id=abc123; ***",
				"Set-Cookie": "session=value; ***",
			},
		},
		{
			name:    "secret redaction combined with masking",
			secrets: map[string]string{"__API_KEY__": "secret123"},
			headers: map[string]string{
				"Authorization": "Bearer secret123",
				"X-Custom-Key":  "safe-value",
			},
			expected: map[string]string{
				"Authorization": "Bearer ***",
				"X-Custom-Key":  "safe-value",
			},
		},
		{
			name:    "already redacted placeholders preserved",
			secrets: map[string]string{"__TOKEN__": "mytoken"},
			headers: map[string]string{
				"Authorization": "Bearer __TOKEN__",
			},
			expected: map[string]string{
				"Authorization": "Bearer ***",
			},
		},
		{
			name:    "case insensitive sensitive header detection",
			secrets: map[string]string{},
			headers: map[string]string{
				"AUTHORIZATION": "Bearer token123",
				"Api-Key":       "key123",
				"x-SECRET":      "secret123",
			},
			expected: map[string]string{
				"AUTHORIZATION": "Bearer ***",
				"Api-Key":       "***",
				"x-SECRET":      "sec***",
			},
		},
		{
			name:    "short values completely masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"X-Token": "abc",
			},
			expected: map[string]string{
				"X-Token": "***",
			},
		},
		{
			name:    "password and credential headers masked",
			secrets: map[string]string{},
			headers: map[string]string{
				"X-Password":   "mypassword123",
				"X-Credential": "cred123",
			},
			expected: map[string]string{
				"X-Password":   "myp***",
				"X-Credential": "***",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			redactor := NewSecretRedactor(tt.secrets)
			result := redactor.RedactHeaders(tt.headers)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsSensitiveHeader(t *testing.T) {
	tests := []struct {
		name      string
		headerKey string
		sensitive bool
	}{
		{"authorization", "Authorization", true},
		{"AUTHORIZATION", "AUTHORIZATION", true},
		{"api-key", "API-Key", true},
		{"x-api-key", "X-API-Key", true},
		{"apikey", "ApiKey", true},
		{"token", "Token", true},
		{"x-token", "X-Token", true},
		{"auth-token", "Auth-Token", true},
		{"access-token", "Access-Token", true},
		{"secret", "Secret", true},
		{"x-secret", "X-Secret", true},
		{"password", "Password", true},
		{"passwd", "Passwd", true},
		{"credential", "Credential", true},
		{"cookie", "Cookie", true},
		{"set-cookie", "Set-Cookie", true},
		{"session", "Session", true},
		{"private-token", "Private-Token", true},
		{"content-type", "Content-Type", false},
		{"accept", "Accept", false},
		{"user-agent", "User-Agent", false},
		{"host", "Host", false},
		{"x-custom-header", "X-Custom-Header", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isSensitiveHeader(tt.headerKey)
			assert.Equal(t, tt.sensitive, result)
		})
	}
}

func TestMaskHeaderValue(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		expected string
	}{
		{
			name:     "empty value",
			value:    "",
			expected: "",
		},
		{
			name:     "bearer token",
			value:    "Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9",
			expected: "Bearer ***",
		},
		{
			name:     "bearer lowercase",
			value:    "bearer my_token_123",
			expected: "bearer ***",
		},
		{
			name:     "basic auth",
			value:    "Basic dXNlcjpwYXNzd29yZA==",
			expected: "Basic ***",
		},
		{
			name:     "basic lowercase",
			value:    "basic credentials",
			expected: "basic ***",
		},
		{
			name:     "already redacted placeholder",
			value:    "__API_KEY__",
			expected: "__API_KEY__",
		},
		{
			name:     "short value",
			value:    "abc123",
			expected: "***",
		},
		{
			name:     "very short value",
			value:    "abc",
			expected: "***",
		},
		{
			name:     "medium value",
			value:    "myapikey123",
			expected: "mya***",
		},
		{
			name:     "long value",
			value:    "sk_live_1234567890abcdef",
			expected: "sk_***",
		},
		{
			name:     "value with space prefix",
			value:    "CustomPrefix my-secret-token",
			expected: "CustomPrefix ***",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskHeaderValue(tt.value)
			assert.Equal(t, tt.expected, result)
		})
	}
}
