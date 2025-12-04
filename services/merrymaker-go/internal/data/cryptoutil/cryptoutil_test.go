package cryptoutil

import (
	"encoding/base64"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAESGCMEncryptor_EncryptDecrypt(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	plaintext := []byte("my secret value")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	// Verify it has the v1 prefix
	assert.Contains(t, ciphertext, "v1:")

	// Decrypt and verify
	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestAESGCMEncryptor_BackwardCompatibilityWithNoop(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	// Simulate a secret that was encrypted with NoopEncryptor
	plaintext := []byte("legacy secret value")
	noopCiphertext := noopPrefix + base64.StdEncoding.EncodeToString(plaintext)

	// AES encryptor should be able to decrypt noop-encrypted secrets
	decrypted, err := enc.Decrypt(noopCiphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestAESGCMEncryptor_InvalidKey(t *testing.T) {
	// Key too short
	_, err := NewAESGCMEncryptor([]byte("short"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")

	// Key too long
	_, err = NewAESGCMEncryptor(make([]byte, 64))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be 32 bytes")
}

func TestAESGCMEncryptor_InvalidCiphertext(t *testing.T) {
	key := make([]byte, 32)
	enc, err := NewAESGCMEncryptor(key)
	require.NoError(t, err)

	// Unknown version
	_, err = enc.Decrypt("v2:somedata")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown ciphertext version")

	// Invalid base64
	_, err = enc.Decrypt("v1:!!!invalid!!!")
	require.Error(t, err)

	// Ciphertext too short
	_, err = enc.Decrypt("v1:" + base64.StdEncoding.EncodeToString([]byte("x")))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}

func TestNoopEncryptor_EncryptDecrypt(t *testing.T) {
	enc := NoopEncryptor{}

	plaintext := []byte("test value")
	ciphertext, err := enc.Encrypt(plaintext)
	require.NoError(t, err)

	// Verify it has the noop prefix
	assert.Contains(t, ciphertext, "noop:")

	// Decrypt and verify
	decrypted, err := enc.Decrypt(ciphertext)
	require.NoError(t, err)
	assert.Equal(t, plaintext, decrypted)
}

func TestNoopEncryptor_InvalidCiphertext(t *testing.T) {
	enc := NoopEncryptor{}

	// Missing noop prefix
	_, err := enc.Decrypt("v1:somedata")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid noop ciphertext")
}
