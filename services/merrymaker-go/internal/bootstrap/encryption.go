package bootstrap

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"

	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
)

// CreateEncryptor creates an AES-GCM encryptor from the provided key.
// If the key is a hex string, it decodes it. Otherwise, it hashes the key to get a 32-byte key.
// Returns a noop encryptor if the key is empty or invalid (with warning log).
//
//nolint:ireturn // Returning interface is intentional for encryptor abstraction
func CreateEncryptor(key string, logger *slog.Logger) cryptoutil.Encryptor {
	if key == "" {
		if logger != nil {
			logger.Warn("encryption key is empty, using noop encryptor")
		}
		return &cryptoutil.NoopEncryptor{}
	}

	enc, err := createAESGCMEncryptor(key)
	if err != nil {
		if logger != nil {
			logger.Warn("failed to create encryptor, using noop encryptor", "error", err)
		}
		return &cryptoutil.NoopEncryptor{}
	}

	return enc
}

func createAESGCMEncryptor(key string) (*cryptoutil.AESGCMEncryptor, error) {
	if key == "" {
		return nil, errors.New("encryption key is required")
	}

	// If the key is a hex string, decode it
	var keyBytes []byte
	if decoded, err := hex.DecodeString(key); err == nil && len(decoded) == 32 {
		keyBytes = decoded
	} else {
		// Otherwise, hash the key to get a 32-byte key
		hash := sha256.Sum256([]byte(key))
		keyBytes = hash[:]
	}

	return cryptoutil.NewAESGCMEncryptor(keyBytes)
}
