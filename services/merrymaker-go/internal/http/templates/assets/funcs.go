package assets

import (
	"html/template"

	httpassets "github.com/target/mmk-ui-api/internal/http/assets"
)

// Options configures asset-related template helpers.
type Options struct {
	Resolver    *httpassets.AssetResolver
	DevMode     bool
	CriticalCSS func() string
}

// Funcs returns template helpers for asset resolution and critical CSS embedding.
func Funcs(opts Options) template.FuncMap {
	funcs := template.FuncMap{
		"asset": func(logicalName string) string {
			return httpassets.ResolveAsset(opts.Resolver, logicalName, opts.DevMode)
		},
	}

	funcs["criticalCSS"] = func() template.CSS {
		if opts.CriticalCSS == nil {
			return ""
		}
		// #nosec G203 - Critical CSS is loaded from our own trusted source files at build time
		return template.CSS(opts.CriticalCSS())
	}

	return funcs
}
