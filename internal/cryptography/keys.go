package cryptography

import (
	"crypto/ed25519"
	"crypto/rand"
)

// GenerateKeyPair creates a new Ed25519 public/private key pair.
func GenerateKeyPair() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}
