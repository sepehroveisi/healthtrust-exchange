package identity

// Role identifies an actor's responsibility within a healthcare organization.
type Role string

const (
	RolePatient       Role = "Patient"
	RoleDoctor        Role = "Doctor"
	RoleStaff         Role = "Staff"
	RoleHospitalAdmin Role = "HospitalAdmin"
)

// ActorStatus controls whether an actor may submit new transactions.
type ActorStatus string

const (
	ActorActive    ActorStatus = "Active"
	ActorSuspended ActorStatus = "Suspended"
	ActorRevoked   ActorStatus = "Revoked"
)

// Actor is a trusted binding between an identity, organization, role, and key.
type Actor struct {
	ID             string
	OrganizationID string
	Role           Role
	PublicKey      []byte
	Status         ActorStatus
}
