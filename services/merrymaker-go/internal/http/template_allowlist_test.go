package httpx

import (
	"bytes"
	"testing"
)

// Verifies allowlist pages map to the expected content partials via sectionTmpl.
func TestTemplateHelpers_Allowlist_Mapping(t *testing.T) {
	tr := RequireTemplateRenderer(t)
	if tr == nil {
		return
	}
	cloned, err := tr.t.Clone()
	if err != nil {
		t.Fatalf("clone: %v", err)
	}
	cloned, err = cloned.Parse(`{{define "probe"}}{{ sectionTmpl . }}{{end}}`)
	if err != nil {
		t.Fatalf("parse probe: %v", err)
	}

	cases := map[string]string{
		PageAllowlist:     "allowlist-content",
		PageAllowlistForm: "allowlist-form-content",
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
