package identity

import (
	"crypto/ed25519"
	"fmt"
	"sync"
)

// MemoryRegistry is a concurrency-safe in-memory identity registry for the MVP.
type MemoryRegistry struct {
	mu            sync.RWMutex
	actors        map[string]Actor
	organizations map[string]Organization
}

func NewMemoryRegistry() *MemoryRegistry {
	return &MemoryRegistry{actors: make(map[string]Actor), organizations: make(map[string]Organization)}
}

func (r *MemoryRegistry) RegisterOrganization(organization Organization) error {
	if organization.ID == "" || organization.Name == "" || organization.Status == "" {
		return fmt.Errorf("%w: organization fields are required", ErrInvalidIdentity)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.organizations[organization.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateOrganization, organization.ID)
	}
	r.organizations[organization.ID] = organization
	return nil
}

func (r *MemoryRegistry) RegisterActor(actor Actor) error {
	if actor.ID == "" || actor.OrganizationID == "" || actor.Role == "" || actor.Status == "" || len(actor.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: valid actor fields and Ed25519 public key are required", ErrInvalidIdentity)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.actors[actor.ID]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateActor, actor.ID)
	}
	actor.PublicKey = append([]byte(nil), actor.PublicKey...)
	r.actors[actor.ID] = actor
	return nil
}

func (r *MemoryRegistry) GetActor(actorID string) (Actor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	actor, exists := r.actors[actorID]
	if !exists {
		return Actor{}, fmt.Errorf("%w: %s", ErrActorNotFound, actorID)
	}
	actor.PublicKey = append([]byte(nil), actor.PublicKey...)
	return actor, nil
}

func (r *MemoryRegistry) GetOrganization(organizationID string) (Organization, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	organization, exists := r.organizations[organizationID]
	if !exists {
		return Organization{}, fmt.Errorf("%w: %s", ErrOrganizationNotFound, organizationID)
	}
	return organization, nil
}
