// Package merrymaker provides embedded assets for production builds.
package merrymaker

import "embed"

// Embedded assets for production builds.
// In dev mode (IsDev=true), assets are loaded from disk for hot reloading.
// In production mode (IsDev=false), assets are served from these embedded filesystems.

//go:embed all:frontend/static
var StaticFS embed.FS

//go:embed all:frontend/templates
var TemplateFS embed.FS
