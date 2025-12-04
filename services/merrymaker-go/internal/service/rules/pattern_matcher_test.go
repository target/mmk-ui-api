package rules

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

func TestPatternMatcher_Match(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		domain      string
		pattern     string
		patternType string
		expected    bool
	}{
		// Exact matching tests
		{
			name:        "exact match - same domain",
			domain:      "example.com",
			pattern:     "example.com",
			patternType: model.PatternTypeExact,
			expected:    true,
		},
		{
			name:        "exact match - different domain",
			domain:      "example.com",
			pattern:     "other.com",
			patternType: model.PatternTypeExact,
			expected:    false,
		},
		{
			name:        "exact match - case insensitive",
			domain:      "Example.COM",
			pattern:     "example.com",
			patternType: model.PatternTypeExact,
			expected:    true,
		},

		// Wildcard matching tests
		{
			name:        "wildcard match - subdomain",
			domain:      "sub.example.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expected:    true,
		},
		{
			name:        "wildcard match - deep subdomain",
			domain:      "deep.sub.example.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expected:    true,
		},
		{
			name:        "wildcard match - exact match with base",
			domain:      "example.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expected:    true,
		},
		{
			name:        "wildcard no match - different base",
			domain:      "sub.other.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expected:    false,
		},
		{
			name:        "wildcard no match - partial match",
			domain:      "notexample.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expected:    false,
		},

		// Glob matching tests
		{
			name:        "glob match - simple wildcard",
			domain:      "test.example.com",
			pattern:     "*.example.com",
			patternType: model.PatternTypeGlob,
			expected:    true,
		},
		{
			name:        "glob match - question mark",
			domain:      "a.example.com",
			pattern:     "?.example.com",
			patternType: model.PatternTypeGlob,
			expected:    true,
		},
		{
			name:        "glob no match - question mark multiple chars",
			domain:      "ab.example.com",
			pattern:     "?.example.com",
			patternType: model.PatternTypeGlob,
			expected:    false,
		},
		{
			name:        "glob match - character class",
			domain:      "1.example.com",
			pattern:     "[0-9].example.com",
			patternType: model.PatternTypeGlob,
			expected:    true,
		},

		// eTLD+1 matching tests
		{
			name:        "etld+1 match - same domain",
			domain:      "example.com",
			pattern:     "example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    true,
		},
		{
			name:        "etld+1 match - subdomain",
			domain:      "sub.example.com",
			pattern:     "example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    true,
		},
		{
			name:        "etld+1 match - deep subdomain",
			domain:      "deep.sub.example.com",
			pattern:     "example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    true,
		},
		{
			name:        "etld+1 no match - different domain",
			domain:      "other.com",
			pattern:     "example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    false,
		},
		{
			name:        "etld+1 match - reverse (pattern has subdomain)",
			domain:      "example.com",
			pattern:     "sub.example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    true,
		},

		// Edge cases
		{
			name:        "empty domain",
			domain:      "",
			pattern:     "example.com",
			patternType: model.PatternTypeExact,
			expected:    false,
		},
		{
			name:        "empty pattern",
			domain:      "example.com",
			pattern:     "",
			patternType: model.PatternTypeExact,
			expected:    false,
		},
		{
			name:        "unknown pattern type defaults to exact",
			domain:      "example.com",
			pattern:     "example.com",
			patternType: "unknown",
			expected:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.Match(tt.domain, tt.pattern, tt.patternType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPatternMatcher_MatchAny(t *testing.T) {
	pm := NewPatternMatcher()

	patterns := []model.DomainAllowlist{
		{Pattern: "allowed.com", PatternType: model.PatternTypeExact, Enabled: true},
		{Pattern: "*.example.com", PatternType: model.PatternTypeWildcard, Enabled: true},
		{Pattern: "test.org", PatternType: model.PatternTypeETLDPlusOne, Enabled: true},
		{Pattern: "disabled.com", PatternType: model.PatternTypeExact, Enabled: false}, // Disabled
	}

	tests := []struct {
		name     string
		domain   string
		expected bool
	}{
		{
			name:     "matches exact pattern",
			domain:   "allowed.com",
			expected: true,
		},
		{
			name:     "matches wildcard pattern",
			domain:   "sub.example.com",
			expected: true,
		},
		{
			name:     "matches etld+1 pattern",
			domain:   "sub.test.org",
			expected: true,
		},
		{
			name:     "disabled pattern not matched",
			domain:   "disabled.com",
			expected: false,
		},
		{
			name:     "no match",
			domain:   "other.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.MatchAny(tt.domain, patterns)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPatternMatcher_ExtractETLDPlusOne(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name     string
		domain   string
		expected string
	}{
		{
			name:     "simple domain",
			domain:   "example.com",
			expected: "example.com",
		},
		{
			name:     "subdomain",
			domain:   "sub.example.com",
			expected: "example.com",
		},
		{
			name:     "deep subdomain",
			domain:   "deep.sub.example.com",
			expected: "example.com",
		},
		{
			name:     "single label",
			domain:   "localhost",
			expected: "",
		},
		{
			name:     "empty domain",
			domain:   "",
			expected: "",
		},
		{
			name:     "multi-part TLD (publicsuffix)",
			domain:   "example.co.uk",
			expected: "example.co.uk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.extractETLDPlusOne(tt.domain)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestPatternMatcher_ValidatePattern(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		pattern     string
		patternType string
		expectError bool
	}{
		{
			name:        "valid exact pattern",
			pattern:     "example.com",
			patternType: model.PatternTypeExact,
			expectError: false,
		},
		{
			name:        "valid wildcard pattern",
			pattern:     "*.example.com",
			patternType: model.PatternTypeWildcard,
			expectError: false,
		},
		{
			name:        "valid glob pattern",
			pattern:     "*.example.com",
			patternType: model.PatternTypeGlob,
			expectError: false,
		},
		{
			name:        "valid etld+1 pattern",
			pattern:     "example.com",
			patternType: model.PatternTypeETLDPlusOne,
			expectError: false,
		},
		{
			name:        "invalid glob pattern",
			pattern:     "[invalid",
			patternType: model.PatternTypeGlob,
			expectError: true,
		},
		{
			name:        "empty pattern",
			pattern:     "",
			patternType: model.PatternTypeExact,
			expectError: false, // Empty patterns are handled elsewhere
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := pm.ValidatePattern(tt.pattern, tt.patternType)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatternMatcher_GetPatternPriority(t *testing.T) {
	pm := NewPatternMatcher()

	tests := []struct {
		name        string
		patternType string
		expected    int
	}{
		{
			name:        "exact has highest priority",
			patternType: model.PatternTypeExact,
			expected:    1,
		},
		{
			name:        "wildcard has second priority",
			patternType: model.PatternTypeWildcard,
			expected:    2,
		},
		{
			name:        "etld+1 has third priority",
			patternType: model.PatternTypeETLDPlusOne,
			expected:    3,
		},
		{
			name:        "glob has fourth priority",
			patternType: model.PatternTypeGlob,
			expected:    4,
		},
		{
			name:        "unknown has lowest priority",
			patternType: "unknown",
			expected:    5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := pm.GetPatternPriority(tt.patternType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMatchETLDPlusOne_PublicSuffix(t *testing.T) {
	pm := NewPatternMatcher()

	// .com
	assert.True(t, pm.Match("sub.example.com", "example.com", model.PatternTypeETLDPlusOne))
	assert.False(t, pm.Match("other.com", "example.com", model.PatternTypeETLDPlusOne))

	// Multi-part TLD (.co.uk) should match only same eTLD+1
	assert.True(t, pm.Match("sub.example.co.uk", "example.co.uk", model.PatternTypeETLDPlusOne))
	assert.False(t, pm.Match("other.co.uk", "example.co.uk", model.PatternTypeETLDPlusOne))
}
