package app

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
)

// KeyProvider is the seam a KMS or Vault can replace: it wraps per-record data
// keys and derives purpose-bound secrets (blind index, magic-link signing).
type KeyProvider interface {
	Wrap(dek []byte) ([]byte, error)
	Unwrap(wrapped []byte) ([]byte, error)
	Derive(purpose string) []byte
}

// envKeyProvider holds one 32-byte key-encryption key from the environment.
type envKeyProvider struct{ kek []byte }

func NewEnvKeyProvider(b64 string) (KeyProvider, error) {
	k, err := base64.StdEncoding.DecodeString(b64)
	if err != nil || len(k) != 32 {
		return nil, errors.New("DATA_KEY must be 32 bytes, base64-encoded")
	}
	return envKeyProvider{k}, nil
}

func (p envKeyProvider) Wrap(dek []byte) ([]byte, error) { return gcmSeal(p.kek, dek) }
func (p envKeyProvider) Unwrap(w []byte) ([]byte, error) { return gcmOpen(p.kek, w) }
func (p envKeyProvider) Derive(purpose string) []byte {
	m := hmac.New(sha256.New, p.kek)
	m.Write([]byte(purpose))
	return m.Sum(nil)
}

func gcmSeal(key, plain []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, g.NonceSize())
	rand.Read(nonce)
	return g.Seal(nonce, nonce, plain, nil), nil
}

func gcmOpen(key, sealed []byte) ([]byte, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	g, err := cipher.NewGCM(b)
	if err != nil {
		return nil, err
	}
	if len(sealed) < g.NonceSize() {
		return nil, errors.New("ciphertext too short")
	}
	return g.Open(nil, sealed[:g.NonceSize()], sealed[g.NonceSize():], nil)
}

// seal encrypts with a fresh data key and stores that key wrapped alongside:
// [1 byte wrapped length][wrapped DEK][ciphertext]. The wrapped DEK is < 256 bytes.
func seal(kp KeyProvider, plain []byte) ([]byte, error) {
	dek := make([]byte, 32)
	rand.Read(dek)
	w, err := kp.Wrap(dek)
	if err != nil {
		return nil, err
	}
	ct, err := gcmSeal(dek, plain)
	if err != nil {
		return nil, err
	}
	return append(append([]byte{byte(len(w))}, w...), ct...), nil
}

func unseal(kp KeyProvider, sealed []byte) ([]byte, error) {
	if len(sealed) < 1 || len(sealed) < 1+int(sealed[0]) {
		return nil, fmt.Errorf("malformed envelope")
	}
	n := int(sealed[0])
	dek, err := kp.Unwrap(sealed[1 : 1+n])
	if err != nil {
		return nil, err
	}
	return gcmOpen(dek, sealed[1+n:])
}

// blindIndex is a keyed hash for lookup and uniqueness without revealing the value.
func blindIndex(kp KeyProvider, v string) []byte {
	m := hmac.New(sha256.New, kp.Derive("blind-index"))
	m.Write([]byte(v))
	return m.Sum(nil)
}
