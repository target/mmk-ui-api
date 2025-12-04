//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

func isLowerHexRune(char rune) bool {
	return ('0' <= char && char <= '9') || ('a' <= char && char <= 'f')
}

// DefaultScope is the default scope value used when no scope is specified.
const DefaultScope = "default"

// ProcessedFile represents a file that has been processed by YARA rules for a specific site and scope.
type ProcessedFile struct {
	ID          string          `json:"id"                     db:"id"`
	SiteID      string          `json:"site_id"                db:"site_id"`
	FileHash    string          `json:"file_hash"              db:"file_hash"`
	StorageKey  string          `json:"storage_key"            db:"storage_key"`
	Scope       string          `json:"scope"                  db:"scope"`
	YaraResults json.RawMessage `json:"yara_results,omitempty" db:"yara_results"`
	ProcessedAt time.Time       `json:"processed_at"           db:"processed_at"`
	CreatedAt   time.Time       `json:"created_at"             db:"created_at"`
}

// CreateProcessedFileRequest represents a request to create a new processed file record.
type CreateProcessedFileRequest struct {
	SiteID      string          `json:"site_id"`
	FileHash    string          `json:"file_hash"`
	StorageKey  string          `json:"storage_key"`
	Scope       string          `json:"scope,omitempty"`
	YaraResults json.RawMessage `json:"yara_results,omitempty"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
}

// Normalize normalizes the CreateProcessedFileRequest fields.
func (r *CreateProcessedFileRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.FileHash = strings.TrimSpace(strings.ToLower(r.FileHash))
	r.StorageKey = strings.TrimSpace(r.StorageKey)
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = DefaultScope
	}
}

// Validate validates the CreateProcessedFileRequest fields.
func (r *CreateProcessedFileRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if r.FileHash == "" {
		return errors.New("file_hash is required")
	}

	// Validate SHA256 hash format (64 hex characters)
	if len(r.FileHash) != 64 {
		return errors.New("file_hash must be a 64-character SHA256 hash")
	}

	// Check if hash contains only hex characters
	for _, char := range r.FileHash {
		if !isLowerHexRune(char) {
			return errors.New("file_hash must contain only hexadecimal characters")
		}
	}

	if r.StorageKey == "" {
		return errors.New("storage_key is required")
	}

	return nil
}

// UpdateProcessedFileRequest represents a request to update an existing processed file record.
type UpdateProcessedFileRequest struct {
	YaraResults json.RawMessage `json:"yara_results,omitempty"`
	ProcessedAt *time.Time      `json:"processed_at,omitempty"`
}

// HasUpdates reports whether any field is set in UpdateProcessedFileRequest.
func (r *UpdateProcessedFileRequest) HasUpdates() bool {
	return r.YaraResults != nil || r.ProcessedAt != nil
}

// Validate validates UpdateProcessedFileRequest, ensuring at least one field is set.
func (r *UpdateProcessedFileRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}

	return nil
}

// ProcessedFileListOptions represents options for listing processed files.
type ProcessedFileListOptions struct {
	SiteID   *string `json:"site_id,omitempty"`
	Scope    *string `json:"scope,omitempty"`
	FileHash *string `json:"file_hash,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	Offset   int     `json:"offset,omitempty"`
}

// ProcessedFileLookupRequest represents a request to check if a file has been processed.
type ProcessedFileLookupRequest struct {
	SiteID   string `json:"site_id"`
	FileHash string `json:"file_hash"`
	Scope    string `json:"scope,omitempty"`
}

// Normalize normalizes the ProcessedFileLookupRequest fields.
func (r *ProcessedFileLookupRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.FileHash = strings.TrimSpace(strings.ToLower(r.FileHash))
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = DefaultScope
	}
}

// Validate validates the ProcessedFileLookupRequest fields.
func (r *ProcessedFileLookupRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if r.FileHash == "" {
		return errors.New("file_hash is required")
	}

	// Validate SHA256 hash format
	if len(r.FileHash) != 64 {
		return errors.New("file_hash must be a 64-character SHA256 hash")
	}
	// Check if hash contains only hex characters (Normalize lowercases input before Validate)
	for _, char := range r.FileHash {
		if !isLowerHexRune(char) {
			return errors.New("file_hash must contain only hexadecimal characters")
		}
	}

	return nil
}

// YaraRuleMatch represents a single YARA rule match result.
type YaraRuleMatch struct {
	RuleName  string            `json:"rule_name"`
	Namespace string            `json:"namespace,omitempty"`
	Tags      []string          `json:"tags,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	Strings   []YaraStringMatch `json:"strings,omitempty"`
}

// YaraStringMatch represents a string match within a YARA rule result.
type YaraStringMatch struct {
	Name   string `json:"name"`
	Offset int64  `json:"offset"`
	Length int    `json:"length"`
	Value  string `json:"value,omitempty"`
}

// YaraResults represents the complete results of YARA rule processing.
type YaraResults struct {
	Matches     []YaraRuleMatch `json:"matches,omitempty"`
	ProcessedAt time.Time       `json:"processed_at"`
	RuleFiles   []string        `json:"rule_files,omitempty"`
	FileSize    int64           `json:"file_size,omitempty"`
	ProcessTime int64           `json:"process_time_ms,omitempty"` // Processing time in milliseconds
	Errors      []string        `json:"errors,omitempty"`
}

// HasMatches returns true if there are any YARA rule matches.
func (r *YaraResults) HasMatches() bool {
	return len(r.Matches) > 0
}

// GetMatchCount returns the total number of YARA rule matches.
func (r *YaraResults) GetMatchCount() int {
	return len(r.Matches)
}

// GetRuleNames returns a slice of all matched rule names.
func (r *YaraResults) GetRuleNames() []string {
	var names []string
	for _, match := range r.Matches {
		names = append(names, match.RuleName)
	}
	return names
}

// ProcessedFileStats represents statistics about processed files in the system.
type ProcessedFileStats struct {
	Total       int `json:"total"`
	WithMatches int `json:"with_matches"`
	NoMatches   int `json:"no_matches"`
	Errors      int `json:"errors"`
}
