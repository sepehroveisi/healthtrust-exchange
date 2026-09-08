package authorityledger

import "time"

type Commitment [32]byte
type Address [20]byte
type OrganizationID string
type AuthorityID string
type SubjectID string
type SeriesID string
type AssertionID string
type ResponseID string
type ResponseVersionID string
type OperationID string

type Organization struct {
	ID                OrganizationID
	Type, DisplayName string
	Active            bool
	Signer            Address
}
type Authority struct {
	ID                AuthorityID
	Type, DisplayName string
	Active            bool
	Signer            Address
}
type ProfessionalSubject struct {
	ID          SubjectID
	DisplayName string
	Details     []byte
}
type EventSeries struct {
	ID          SeriesID
	SubjectID   SubjectID
	AuthorityID AuthorityID
}
type AuthorityAssertion struct {
	ID                      AssertionID
	SeriesID                SeriesID
	PreviousID, TargetID    *AssertionID
	EventType, Kind, Effect string
	EffectiveUnixNano       int64
	SourceDocument          []byte
	Commitment              Commitment
	CanonicalVersion        int16
	CreatedAt               time.Time
}
type Policy struct {
	ID, VersionID          string
	OrganizationID         OrganizationID
	EffectiveFromUnixNano  int64
	EffectiveUntilUnixNano *int64
	Text                   string
	Commitment             Commitment
	CanonicalVersion       int16
}
type Decision struct {
	ID                                                    string
	EventID                                               AssertionID
	OrganizationID                                        OrganizationID
	PolicyVersionID, Code, Explanation, ReviewerReference string
	DecidedUnixNano                                       int64
	Commitment                                            Commitment
	CanonicalVersion                                      int16
}
type Action struct {
	ID                                               string
	EventID                                          AssertionID
	OrganizationID                                   OrganizationID
	DecisionID, Code, Explanation, OperatorReference string
	CompletedUnixNano                                int64
	SupportingEvidence                               []byte
	Commitment                                       Commitment
	CanonicalVersion                                 int16
}
type ResponseStream struct {
	ID             ResponseID
	EventID        AssertionID
	OrganizationID OrganizationID
}
type ResponseVersion struct {
	ID                                    ResponseVersionID
	ResponseID                            ResponseID
	EventID                               AssertionID
	OrganizationID                        OrganizationID
	PreviousID                            *ResponseVersionID
	State                                 string
	ReceiptUnixNano                       int64
	PolicyVersionID, DecisionID, ActionID *string
	Commitment                            Commitment
	CanonicalVersion                      int16
	CreatedAt                             time.Time
}
type Submission struct {
	OperationID                                                   OperationID
	OperationType, DomainVersionID, ContractAction, SignerContext string
	IntendedCommitment                                            Commitment
	State                                                         string
	TransactionHash, BlockHash                                    []byte
	BlockNumber                                                   *int64
	AttemptCount                                                  int
	LastAttemptAt                                                 *time.Time
	LastErrorClass                                                *string
	CreatedAt, UpdatedAt                                          time.Time
}
