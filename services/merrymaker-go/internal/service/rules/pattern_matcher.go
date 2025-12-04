package rules

import (
	"path/filepath"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"golang.org/x/net/publicsuffix"
)

// PatternMatcher provides domain pattern matching functionality for allowlists.
type PatternMatcher struct{}

// NewPatternMatcher creates a new pattern matcher.
func NewPatternMatcher() *PatternMatcher {
	return &PatternMatcher{}
}

// Match checks if a domain matches a given pattern based on the pattern type.
func (pm *PatternMatcher) Match(domain, pattern, patternType string) bool {
	// Normalize inputs
	domain = strings.ToLower(strings.TrimSpace(domain))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	patternType = strings.ToLower(strings.TrimSpace(patternType))

	if domain == "" || pattern == "" {
		return false
	}

	switch patternType {
	case model.PatternTypeExact:
		return pm.matchExact(domain, pattern)
	case model.PatternTypeWildcard:
		return pm.matchWildcard(domain, pattern)
	case model.PatternTypeGlob:
		return pm.matchGlob(domain, pattern)
	case model.PatternTypeETLDPlusOne:
		return pm.matchETLDPlusOne(domain, pattern)
	default:
		// Default to exact match for unknown pattern types
		return pm.matchExact(domain, pattern)
	}
}

// matchExact performs exact domain matching.
func (pm *PatternMatcher) matchExact(domain, pattern string) bool {
	return domain == pattern
}

// matchWildcard performs simple wildcard matching (*.example.com).
// Supports single wildcard at the beginning of the pattern.
func (pm *PatternMatcher) matchWildcard(domain, pattern string) bool {
	// Handle exact match first
	if domain == pattern {
		return true
	}

	// Check for wildcard pattern
	if !strings.HasPrefix(pattern, "*.") {
		return false
	}

	// Extract the base domain from pattern (remove "*.")
	baseDomain := pattern[2:]
	if baseDomain == "" {
		return false
	}

	// Domain must end with the base domain
	if !strings.HasSuffix(domain, baseDomain) {
		return false
	}

	// Ensure it's a proper subdomain match (not partial match)
	if len(domain) == len(baseDomain) {
		return true // Exact match with base domain
	}

	// Check that the character before the base domain is a dot
	prefixLen := len(domain) - len(baseDomain)
	return domain[prefixLen-1] == '.'
}

// matchGlob performs glob pattern matching using filepath.Match.
func (pm *PatternMatcher) matchGlob(domain, pattern string) bool {
	matched, err := filepath.Match(pattern, domain)
	if err != nil {
		// If pattern is invalid, fall back to exact match
		return domain == pattern
	}
	return matched
}

// matchETLDPlusOne performs eTLD+1 matching.
// This matches the effective top-level domain plus one level.
// For example, "example.com" matches "sub.example.com", "deep.sub.example.com", etc.
func (pm *PatternMatcher) matchETLDPlusOne(domain, pattern string) bool {
	// Handle exact match first
	if domain == pattern {
		return true
	}

	// Extract eTLD+1 from both domain and pattern
	domainETLD := pm.extractETLDPlusOne(domain)
	patternETLD := pm.extractETLDPlusOne(pattern)

	return domainETLD == patternETLD && domainETLD != ""
}

// extractETLDPlusOne extracts the eTLD+1 from a domain using the public suffix list.
func (pm *PatternMatcher) extractETLDPlusOne(domain string) string {
	domain = strings.TrimSpace(strings.ToLower(domain))
	if domain == "" {
		return ""
	}
	etld1, err := publicsuffix.EffectiveTLDPlusOne(domain)
	if err != nil {
		return ""
	}
	return etld1
}

// MatchAny checks if a domain matches any of the provided patterns.
func (pm *PatternMatcher) MatchAny(domain string, patterns []model.DomainAllowlist) bool {
	for i := range patterns {
		pattern := &patterns[i]
		if !pattern.Enabled {
			continue
		}
		if pm.Match(domain, pattern.Pattern, pattern.PatternType) {
			return true
		}
	}
	return false
}

// ValidatePattern validates that a pattern is syntactically correct for its type.
func (pm *PatternMatcher) ValidatePattern(pattern, patternType string) error {
	pattern = strings.TrimSpace(pattern)
	patternType = strings.ToLower(strings.TrimSpace(patternType))

	if pattern == "" {
		return nil // Empty patterns are handled elsewhere
	}

	switch patternType {
	case model.PatternTypeExact:
		return pm.validateExactPattern(pattern)
	case model.PatternTypeWildcard:
		return pm.validateWildcardPattern(pattern)
	case model.PatternTypeGlob:
		return pm.validateGlobPattern(pattern)
	case model.PatternTypeETLDPlusOne:
		return pm.validateETLDPlusOnePattern(pattern)
	default:
		return nil // Unknown pattern types are handled elsewhere
	}
}

// validateExactPattern validates exact match patterns.
func (pm *PatternMatcher) validateExactPattern(pattern string) error {
	// For exact patterns, just ensure it looks like a reasonable domain
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		// Suggest using wildcard or glob pattern type instead
		return nil // Don't error, but the pattern might not work as expected
	}
	return nil
}

// validateWildcardPattern validates wildcard patterns.
func (pm *PatternMatcher) validateWildcardPattern(pattern string) error {
	// Wildcard patterns should start with "*." and have a valid base domain
	if !strings.HasPrefix(pattern, "*.") {
		return nil // Allow non-wildcard patterns in wildcard type (they'll match exactly)
	}

	baseDomain := pattern[2:]
	if baseDomain == "" {
		return nil // Empty base domain is technically valid but not useful
	}

	// Ensure no additional wildcards in the base domain
	if strings.Contains(baseDomain, "*") {
		return nil // Multiple wildcards not supported in simple wildcard mode
	}

	return nil
}

// validateGlobPattern validates glob patterns.
func (pm *PatternMatcher) validateGlobPattern(pattern string) error {
	// Test the pattern with filepath.Match to ensure it's valid
	_, err := filepath.Match(pattern, "test.example.com")
	return err
}

// validateETLDPlusOnePattern validates eTLD+1 patterns.
func (pm *PatternMatcher) validateETLDPlusOnePattern(pattern string) error {
	// eTLD+1 patterns should look like valid domains
	if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
		return nil // Wildcards don't make sense in eTLD+1 mode
	}

	parts := strings.Split(pattern, ".")
	if len(parts) < 2 {
		return nil // Single label domains are technically valid
	}

	return nil
}

// GetPatternPriority returns a priority score for pattern model.
// Lower numbers indicate higher priority (more specific matches).
func (pm *PatternMatcher) GetPatternPriority(patternType string) int {
	switch strings.ToLower(patternType) {
	case model.PatternTypeExact:
		return 1 // Highest priority - exact matches
	case model.PatternTypeWildcard:
		return 2 // Second priority - simple wildcards
	case model.PatternTypeETLDPlusOne:
		return 3 // Third priority - eTLD+1 matches
	case model.PatternTypeGlob:
		return 4 // Lowest priority - complex globs
	default:
		return 5 // Unknown pattern types get lowest priority
	}
}
