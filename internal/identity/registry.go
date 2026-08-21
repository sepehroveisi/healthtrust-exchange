package identity

import "errors"

var (
	ErrActorNotFound         = errors.New("actor not found")
	ErrOrganizationNotFound  = errors.New("organization not found")
	ErrDuplicateActor        = errors.New("actor already registered")
	ErrDuplicateOrganization = errors.New("organization already registered")
	ErrInvalidIdentity       = errors.New("invalid identity")
)

// Registry resolves transaction identity claims against trusted records.
type Registry interface {
	GetActor(actorID string) (Actor, error)
	GetOrganization(organizationID string) (Organization, error)
}
