package workflow

import (
	"crypto/ed25519"
	"crypto/sha256"
	"strings"
	"testing"
)

func TestDiscoveryScopeIsOpaqueAndBoundToPatientAndRequester(t *testing.T) {
	seed := sha256.Sum256([]byte("discovery-node"))
	service := Service{NodeKey: ed25519.NewKeyFromSeed(seed[:])}
	scope := service.discoveryScope("network-jean", "private-record-id", "hospital-b")
	if strings.Contains(scope, "network-jean") || strings.Contains(scope, "private-record-id") {
		t.Fatal("scope leaked a private identifier")
	}
	if scope == service.discoveryScope("network-bruce", "private-record-id", "hospital-b") {
		t.Fatal("scope was not patient-bound")
	}
	if scope == service.discoveryScope("network-jean", "private-record-id", "hospital-c") {
		t.Fatal("scope was not requester-bound")
	}
	if scope == service.discoveryScope("network-jean", "other-record", "hospital-b") {
		t.Fatal("scope was not record-bound")
	}
}
