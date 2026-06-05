package server

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
)

func (a *App) encryptSecret(plain []byte) (string, error) {
	key := sha256.Sum256([]byte(a.cfg.SecretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, plain, nil)
	return base64.RawURLEncoding.EncodeToString(sealed), nil
}

func (a *App) decryptSecret(ciphertext string) ([]byte, error) {
	if ciphertext == "" {
		return []byte("{}"), nil
	}
	blob, err := base64.RawURLEncoding.DecodeString(ciphertext)
	if err != nil {
		return nil, err
	}
	key := sha256.Sum256([]byte(a.cfg.SecretKey))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(blob) < gcm.NonceSize() {
		return nil, fmt.Errorf("secret ciphertext too short")
	}
	nonce, payload := blob[:gcm.NonceSize()], blob[gcm.NonceSize():]
	return gcm.Open(nil, nonce, payload, nil)
}
