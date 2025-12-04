package cryptoutil

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Encryptor defines an interface for encrypting/decrypting secrets.
type Encryptor interface {
	Encrypt(plaintext []byte) (string, error)
	Decrypt(ciphertext string) ([]byte, error)
}

// AESGCMEncryptor implements Encryptor using AES-256-GCM.
type AESGCMEncryptor struct {
	key []byte // 32 bytes
}

const (
	// Versioned prefix to allow future key/algorithm rotations without data migrations.
	secretCipherPrefixV1 = "v1:"
	noopPrefix           = "noop:"
)

// NewAESGCMEncryptor constructs a new AESGCMEncryptor. Key must be 32 bytes (AES-256).
func NewAESGCMEncryptor(key []byte) (*AESGCMEncryptor, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("aes-gcm key must be 32 bytes, got %d", len(key))
	}
	return &AESGCMEncryptor{key: append([]byte(nil), key...)}, nil
}

// Encrypt encrypts plaintext with a random nonce and returns a versioned base64 string.
func (e *AESGCMEncryptor) Encrypt(plaintext []byte) (string, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, readErr := io.ReadFull(rand.Reader, nonce); readErr != nil {
		return "", readErr
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	// Store nonce||ciphertext
	buf := make([]byte, 0, len(nonce)+len(ct))
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	return secretCipherPrefixV1 + base64.StdEncoding.EncodeToString(buf), nil
}

// Decrypt decrypts a versioned base64 string created by Encrypt.
// Supports backward compatibility with noop-encrypted secrets (for migration scenarios).
func (e *AESGCMEncryptor) Decrypt(ciphertext string) ([]byte, error) {
	// Backward compatibility: handle noop-encrypted secrets from before encryption key was configured
	if strings.HasPrefix(ciphertext, noopPrefix) {
		decoded, err := base64.StdEncoding.DecodeString(ciphertext[len(noopPrefix):])
		if err != nil {
			return nil, fmt.Errorf("decode noop ciphertext: %w", err)
		}
		return decoded, nil
	}

	if !strings.HasPrefix(ciphertext, secretCipherPrefixV1) {
		// Log the prefix for debugging
		var prefix string
		if len(ciphertext) > 10 {
			prefix = ciphertext[:10]
		} else {
			prefix = ciphertext
		}
		return nil, fmt.Errorf("unknown ciphertext version (prefix: %s)", prefix)
	}
	b64 := ciphertext[len(secretCipherPrefixV1):]
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}
	nonce, ct := data[:nonceSize], data[nonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, err
	}
	return pt, nil
}

// NoopEncryptor is useful for tests; it stores plaintext with a prefix marker.
type NoopEncryptor struct{}

func (NoopEncryptor) Encrypt(plaintext []byte) (string, error) {
	return noopPrefix + base64.StdEncoding.EncodeToString(plaintext), nil
}

func (NoopEncryptor) Decrypt(ciphertext string) ([]byte, error) {
	if !strings.HasPrefix(ciphertext, noopPrefix) {
		return nil, errors.New("invalid noop ciphertext")
	}
	return base64.StdEncoding.DecodeString(ciphertext[len(noopPrefix):])
}
