//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

// Package name "types" is shared across core domain models for compatibility.

// SourceJobCounts holds aggregated job counts for a Source.
type SourceJobCounts struct {
	Total   int
	Browser int
}
