package httpx

import (
	"bytes"
	"testing"
)

// Ensures sectionTmpl maps known pages to expected partials and defaults for unknown.
func TestTemplateHelpers_SectionTmpl_Mapping(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	// Clone the template set and add a tiny probe template that calls sectionTmpl
	cloned, err := tr.t.Clone()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloned, err = cloned.Parse(`{{define "probe"}}{{ sectionTmpl . }}{{end}}`)
	if err != nil {
		t.Fatalf("parse probe: %v", err)
	}

	cases := map[string]string{
		PageHome:       "dashboard-content", // Home page now shows dashboard
		PageDashboard:  "dashboard-content",
		PageAlerts:     "alerts-content",
		PageAlert:      "alert-sink-view-content", // backward-compat key maps to sink view
		PageAlertSinks: "alert-sinks-content",
		"unknown":      "dashboard-content", // fallback to dashboard
	}
	for page, want := range cases {
		var buf bytes.Buffer
		if err := cloned.ExecuteTemplate(&buf, "probe", page); err != nil {
			t.Fatalf("execute probe(%s): %v", page, err)
		}
		got := buf.String()
		if got != want {
			t.Fatalf("sectionTmpl(%s) => %q, want %q", page, got, want)
		}
	}
}

// Ensures renderSection renders the correct partial and falls back on unknown pages.
func TestTemplateHelpers_RenderSection_RendersAndFallbacks(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}

	// Add a probe that prints the rendered HTML from renderSection
	cloned, err := tr.t.Clone()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloned, err = cloned.Parse(`{{define "probe"}}{{ renderSection .Page .Data }}{{end}}`)
	if err != nil {
		t.Fatalf("parse probe: %v", err)
	}

	t.Run("dashboard renders expected content", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{
			"Page": PageDashboard,
			"Data": map[string]any{},
		}
		if err := cloned.ExecuteTemplate(&buf, "probe", data); err != nil {
			t.Fatalf("execute probe(dashboard): %v", err)
		}
		html := buf.String()
		if !containsAll(html, []string{"recent-browser-jobs-panel", "Recent Browser Jobs"}) {
			t.Fatalf("dashboard render missing expected substrings: %q", html)
		}
	})

	t.Run("unknown page falls back to dashboard", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]any{"Page": "nope", "Data": map[string]any{}}
		if err := cloned.ExecuteTemplate(&buf, "probe", data); err != nil {
			t.Fatalf("execute probe(unknown): %v", err)
		}
		html := buf.String()
		if !containsAll(html, []string{"stats-grid"}) {
			t.Fatalf("fallback render missing 'stats-grid': %q", html)
		}
	})
}

// containsAll is now available as ContainsAll in testhelpers.go
// This function is kept for backward compatibility in this test file.
func containsAll(s string, subs []string) bool {
	return ContainsAll(s, subs)
}
