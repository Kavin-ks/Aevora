// Package auth holds small credential helpers shared by the store and the API.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

// GenerateToken returns a new, cryptographically random opaque token (the
// plaintext shown to the caller exactly once). 32 bytes → 256 bits of entropy.
func GenerateToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// HashToken returns the hex SHA-256 of a token. Only the hash is stored; the
// plaintext is never persisted, so a database leak does not expose node tokens.
func HashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
