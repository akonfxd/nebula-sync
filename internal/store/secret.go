package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	plainPrefix  = "plain:"
	sealedPrefix = "aesgcm:"
	keyLength    = 32
)

var errNoKey = errors.New("stored secret is encrypted but no encryption key is configured")

type secretCipher struct {
	aead cipher.AEAD
}

func newSecretCipher(key string) (*secretCipher, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return &secretCipher{}, nil
	}

	raw, err := decodeKey(key)
	if err != nil {
		return nil, err
	}

	block, err := aes.NewCipher(raw)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("encryption key: %w", err)
	}

	return &secretCipher{aead: aead}, nil
}

func decodeKey(key string) ([]byte, error) {
	if raw, err := hex.DecodeString(key); err == nil && len(raw) == keyLength {
		return raw, nil
	}
	if raw, err := base64.StdEncoding.DecodeString(key); err == nil && len(raw) == keyLength {
		return raw, nil
	}
	if raw, err := base64.RawStdEncoding.DecodeString(key); err == nil && len(raw) == keyLength {
		return raw, nil
	}

	return nil, fmt.Errorf("encryption key must be %d bytes, hex or base64 encoded", keyLength)
}

func (c *secretCipher) enabled() bool {
	return c.aead != nil
}

func (c *secretCipher) seal(value string) (string, error) {
	if !c.enabled() {
		return plainPrefix + value, nil
	}

	nonce := make([]byte, c.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	return sealedPrefix + base64.StdEncoding.EncodeToString(c.aead.Seal(nonce, nonce, []byte(value), nil)), nil
}

func (c *secretCipher) open(stored string) (string, error) {
	switch {
	case strings.HasPrefix(stored, plainPrefix):
		return strings.TrimPrefix(stored, plainPrefix), nil
	case strings.HasPrefix(stored, sealedPrefix):
		return c.unseal(strings.TrimPrefix(stored, sealedPrefix))
	default:
		return "", errors.New("stored secret has an unknown encoding")
	}
}

func (c *secretCipher) unseal(encoded string) (string, error) {
	if !c.enabled() {
		return "", errNoKey
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}

	if len(raw) < c.aead.NonceSize() {
		return "", errors.New("stored secret is truncated")
	}

	nonce, ciphertext := raw[:c.aead.NonceSize()], raw[c.aead.NonceSize():]

	value, err := c.aead.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt secret: %w", err)
	}

	return string(value), nil
}
