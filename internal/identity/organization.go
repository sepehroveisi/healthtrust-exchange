package identity

// OrganizationStatus controls whether an organization is trusted by the registry.
type OrganizationStatus string

const (
	OrganizationActive   OrganizationStatus = "Active"
	OrganizationInactive OrganizationStatus = "Inactive"
)

// Organization represents a healthcare organization in the permissioned network.
type Organization struct {
	ID     string
	Name   string
	Status OrganizationStatus
}
