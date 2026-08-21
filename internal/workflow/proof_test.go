package workflow

import (
	"crypto/ed25519"
	"crypto/sha256"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"testing"
	"time"
)

func TestRequesterProofRejectsForgery(t *testing.T) {
	seed := sha256.Sum256([]byte("doctor"))
	key := ed25519.NewKeyFromSeed(seed[:])
	registry := identity.NewMemoryRegistry()
	_ = registry.RegisterOrganization(identity.Organization{ID: "hospital-b", Name: "B", Status: identity.OrganizationActive})
	_ = registry.RegisterActor(identity.Actor{ID: "doctor-b", OrganizationID: "hospital-b", Role: identity.RoleDoctor, PublicKey: key.Public().(ed25519.PublicKey), Status: identity.ActorActive})
	now := time.Now().UTC()
	valid := SignRequesterProof("doctor-b", "hospital-b", "record", now, key)
	if err := VerifyRequesterProof(valid, registry, now); err != nil {
		t.Fatal(err)
	}
	attackerSeed := sha256.Sum256([]byte("attacker"))
	attacker := ed25519.NewKeyFromSeed(attackerSeed[:])
	forged := SignRequesterProof("doctor-b", "hospital-b", "record", now, attacker)
	if err := VerifyRequesterProof(forged, registry, now); err == nil {
		t.Fatal("forged proof accepted")
	}
}
