package httpx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTemplateRenderer_LoadTemplates(t *testing.T) {
	// Test loading templates from the correct path
	tr := RequireTemplateRenderer(t)
	require.NotNil(t, tr, "Template renderer should not be nil")

	// Test that the templates are loaded correctly
	assert.NotNil(t, tr.t, "Template should be loaded")

	// Test that we can find the expected templates
	templates := tr.t.Templates()
	templateNames := make([]string, len(templates))
	for i, tmpl := range templates {
		templateNames[i] = tmpl.Name()
	}

	// Check for expected template names
	expectedTemplates := []string{"layout", "content", "error-layout", "dashboard-content"}
	for _, expected := range expectedTemplates {
		found := false
		for _, name := range templateNames {
			if name == expected {
				found = true
				break
			}
		}
		assert.True(t, found, "Template %s should be loaded", expected)
	}
}

func TestTemplateRenderer_FromCurrentDir(t *testing.T) {
	// Test loading templates from the current directory (as the router does)
	tr := RequireTemplateRendererFromRoot(t)
	if tr == nil {
		return
	}

	assert.NotNil(t, tr, "Template renderer should not be nil")
	assert.NotNil(t, tr.t, "Template should be loaded")
}
