package nvr

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
)

const cipherPrefix = "v1:"

func tokenKey(secretKey string) []byte {
	sum := sha256.Sum256([]byte("nvr-agent-token:" + secretKey))
	return sum[:]
}

// EncryptToken seals an agent token with a key derived from the platform secret.
func EncryptToken(secretKey, token string) (string, error) {
	if token == "" {
		return "", nil
	}
	gcm, err := newGCM(secretKey)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(token), nil)
	return cipherPrefix + base64.RawStdEncoding.EncodeToString(sealed), nil
}

// DecryptToken reverses EncryptToken. An empty cipher means no token.
func DecryptToken(secretKey, cipherText string) (string, error) {
	if cipherText == "" {
		return "", nil
	}
	if len(cipherText) < len(cipherPrefix) || cipherText[:len(cipherPrefix)] != cipherPrefix {
		return "", errors.New("unsupported token cipher")
	}
	raw, err := base64.RawStdEncoding.DecodeString(cipherText[len(cipherPrefix):])
	if err != nil {
		return "", err
	}
	gcm, err := newGCM(secretKey)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("token cipher too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("token cannot be decrypted (platform secret key changed?)")
	}
	return string(plain), nil
}

func newGCM(secretKey string) (cipher.AEAD, error) {
	block, err := aes.NewCipher(tokenKey(secretKey))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}
