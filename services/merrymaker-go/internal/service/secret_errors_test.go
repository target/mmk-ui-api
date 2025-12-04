package service

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSecretProviderScriptError_UserMessage(t *testing.T) {
	err := NewSecretProviderScriptError("/opt/script.sh", errors.New("script failed\nline2"))

	msg := err.UserMessage()
	assert.Contains(t, msg, "Refresh script failed during validation")
	assert.Contains(t, msg, "/opt/script.sh")
	assert.NotContains(t, msg, "\n")
	assert.Contains(t, msg, "script failed")
}
