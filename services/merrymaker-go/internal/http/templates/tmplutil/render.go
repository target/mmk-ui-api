// Package tmplutil provides template helper utilities shared across template sub-packages.
package tmplutil

import (
	"bytes"
	"errors"
	"html/template"
)

// RenderPartial returns a template.FuncMap-compatible closure that executes a named
// sub-template into a buffer and returns the result as trusted template.HTML.
//
// The caller is responsible for ensuring that t always refers to the authoritative
// *template.Template pointer so late-bound template sets (registered after Funcs is
// called) are picked up correctly.
func RenderPartial(t **template.Template) func(string, any) (template.HTML, error) {
	return func(name string, data any) (template.HTML, error) {
		if t == nil || *t == nil {
			return "", errors.New("template not initialized")
		}
		var buf bytes.Buffer
		if err := (*t).ExecuteTemplate(&buf, name, data); err != nil {
			return "", err
		}
		// #nosec G203 - The HTML here is rendered by our own trusted templates (html/template)
		// and is embedded back into the same template set. User-provided values were already
		// auto-escaped during ExecuteTemplate above.
		return template.HTML(buf.String()), nil //nolint:gosec // G203: see comment above
	}
}
