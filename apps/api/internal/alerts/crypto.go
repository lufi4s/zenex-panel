package alerts

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
)

// ErrNoSecretKey means the server secret is not configured, so secrets cannot be stored.
var ErrNoSecretKey = errors.New("ZENEX_SECRET_KEY is not set; passwords and tokens cannot be stored")

// ErrCiphertext means a stored secret could not be decrypted (wrong key or damaged value).
var ErrCiphertext = errors.New("stored secret could not be decrypted")

// DeriveKey turns the server secret into a 256-bit AES key.
func DeriveKey(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

// Seal encrypts plaintext with AES-256-GCM and returns base64(nonce || ciphertext).
func Seal(key []byte, plaintext string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// Open reverses Seal.
func Open(key []byte, encoded string) (string, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", ErrCiphertext
	}
	nonce, body := raw[:gcm.NonceSize()], raw[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, body, nil)
	if err != nil {
		return "", ErrCiphertext
	}
	return string(plain), nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
