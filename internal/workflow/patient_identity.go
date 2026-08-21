package workflow

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

// demoPatientActorID creates an organization-scoped consent signer for the
// prototype. It is not a production MPI, authentication, or key-management design.
func demoPatientActorID(organizationID, networkPatientID string) string {
	// The UI-selected local patient is still checked against the request. This
	// actor is only the organization-scoped demo consent signer shared by peers.
	return "patient-consent-" + organizationID
}

func demoPatientKey(actorID string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte("healthtrust-demo-only-" + actorID))
	return ed25519.NewKeyFromSeed(seed[:])
}

func (s *Service) ensureDemoPatientActor(ctx context.Context, patient clinical.Patient) error {
	actorID := demoPatientActorID(patient.OrganizationID, patient.NetworkPatientID)
	key := demoPatientKey(actorID)
	s.Keys[actorID] = key
	if _, err := s.Registry.GetActor(actorID); err == nil {
		return nil
	}
	registry, ok := s.Registry.(*identity.MemoryRegistry)
	if !ok {
		return errors.New("patient identity registry is not writable")
	}
	actor := identity.Actor{ID: actorID, OrganizationID: patient.OrganizationID, Role: identity.RolePatient, PublicKey: key.Public().(ed25519.PublicKey), Status: identity.ActorActive}
	if err := registry.RegisterActor(actor); err != nil {
		return err
	}
	if store, ok := s.Store.(interface {
		SaveActor(context.Context, identity.Actor) error
	}); ok {
		return store.SaveActor(ctx, actor)
	}
	return nil
}

func (s *Service) patientConsentActor(ctx context.Context, localPatientID string) (string, identity.Actor, error) {
	store, ok := s.Store.(OperationalStore)
	if !ok {
		actor, err := s.Registry.GetActor(localPatientID)
		if err == nil && actor.Role == identity.RolePatient {
			return localPatientID, actor, nil
		}
		return "", identity.Actor{}, ErrAccessDenied
	}
	patient, err := store.GetPatient(ctx, s.OrganizationID, localPatientID)
	if err != nil || patient.NetworkPatientID == "" {
		return "", identity.Actor{}, ErrAccessDenied
	}
	if err := s.ensureDemoPatientActor(ctx, patient); err != nil {
		return "", identity.Actor{}, err
	}
	id := demoPatientActorID(patient.OrganizationID, patient.NetworkPatientID)
	actor, err := s.Registry.GetActor(id)
	return id, actor, err
}
