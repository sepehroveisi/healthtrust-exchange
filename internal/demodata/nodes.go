package demodata

import (
	"crypto/ed25519"
	"crypto/sha256"
)

func NodeKey(nodeID string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("healthtrust-demo-node-only-" + nodeID))
	return ed25519.NewKeyFromSeed(seed[:])
}
