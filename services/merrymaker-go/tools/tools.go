//go:build tools
// +build tools

// Package tools documents development tool dependencies.
// These tools are installed globally via `go install` and are not tracked in go.mod
// since they are development tools, not runtime dependencies.
package tools

// Development tools (install via `go install`):
//
// Air - Live reload for Go apps
//   Install: go install github.com/air-verse/air@v1.63.0
//   Version: v1.63.0 (pinned 2025-01-01)
//   Docs: https://github.com/air-verse/air
