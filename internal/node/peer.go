package node

// Peer describes another permissioned hospital node.
type Peer struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	BaseURL        string `json:"baseUrl"`
}
