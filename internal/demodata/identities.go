package demodata

import (
	"crypto/ed25519"
	"crypto/sha256"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

// Registry returns deterministic fictional identities for local demonstrations only.
func Registry() (*identity.MemoryRegistry, map[string]ed25519.PrivateKey, error) {
	registry := identity.NewMemoryRegistry()
	for _, organization := range []identity.Organization{{ID: "hospital-a", Name: "Hospital A", Status: identity.OrganizationActive}, {ID: "hospital-b", Name: "Hospital B", Status: identity.OrganizationActive}} {
		if err := registry.RegisterOrganization(organization); err != nil {
			return nil, nil, err
		}
	}
	keys := make(map[string]ed25519.PrivateKey)
	actors := []struct {
		id, organization string
		role             identity.Role
	}{{"doctor-a", "hospital-a", identity.RoleDoctor}, {"doctor-b", "hospital-b", identity.RoleDoctor}, {"patient-p", "hospital-a", identity.RolePatient}, {"patient-consent-hospital-a", "hospital-a", identity.RolePatient}, {"patient-consent-hospital-b", "hospital-b", identity.RolePatient}}
	for _, value := range actors {
		seed := sha256.Sum256([]byte("healthtrust-demo-only-" + value.id))
		privateKey := ed25519.NewKeyFromSeed(seed[:])
		keys[value.id] = privateKey
		if err := registry.RegisterActor(identity.Actor{ID: value.id, OrganizationID: value.organization, Role: value.role, PublicKey: privateKey.Public().(ed25519.PublicKey), Status: identity.ActorActive}); err != nil {
			return nil, nil, err
		}
	}
	return registry, keys, nil
}
