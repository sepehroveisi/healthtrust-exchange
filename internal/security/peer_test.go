package security

import (
	"crypto/ed25519"
	"crypto/sha256"
	"testing"
	"time"
)

func TestPeerEnvelopeBindings(t *testing.T) {
	seed := sha256.Sum256([]byte("peer"))
	key := ed25519.NewKeyFromSeed(seed[:])
	now := time.Now().UTC()
	body := []byte("body")
	base := PeerEnvelope{RequestID: "r", SourceNodeID: "b", SourceOrganizationID: "hospital-b", TargetNodeID: "a", HTTPMethod: "POST", RequestPath: "/internal/x", BodyHash: sha256.Sum256(body), IssuedAt: now, ExpiresAt: now.Add(time.Minute), Nonce: "n"}
	signed := SignPeerEnvelope(base, key)
	peer := Peer{NodeID: "b", OrganizationID: "hospital-b", PublicKey: key.Public().(ed25519.PublicKey)}
	if err := VerifyPeerEnvelope(signed, peer, "a", "POST", "/internal/x", body, now); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		change func(*PeerEnvelope)
		body   []byte
	}{{"path", func(e *PeerEnvelope) { e.RequestPath = "/changed" }, body}, {"organization", func(e *PeerEnvelope) { e.SourceOrganizationID = "other" }, body}, {"body", func(e *PeerEnvelope) {}, []byte("changed")}, {"expired", func(e *PeerEnvelope) { e.ExpiresAt = now.Add(-time.Second) }, body}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := signed
			tt.change(&e)
			if err := VerifyPeerEnvelope(e, peer, "a", "POST", "/internal/x", tt.body, now); err == nil {
				t.Fatal("modified envelope accepted")
			}
		})
	}
}
