package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSiteAlertMode(t *testing.T) {
	mode, ok := ParseSiteAlertMode("Muted")
	assert.True(t, ok)
	assert.Equal(t, SiteAlertModeMuted, mode)

	mode, ok = ParseSiteAlertMode(" active ")
	assert.True(t, ok)
	assert.Equal(t, SiteAlertModeActive, mode)

	_, ok = ParseSiteAlertMode("unknown")
	assert.False(t, ok)
}
