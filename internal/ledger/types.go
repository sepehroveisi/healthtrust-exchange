package ledger

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"regexp"
	"time"
)

var idPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9._-]{0,31}$`)

type ID [32]byte
type Address [20]byte
type Commitment [32]byte
type TransactionHash [32]byte
type BlockNumber uint64

func NewID(value string) (ID, error) {
	if !idPattern.MatchString(value) {
		return ID{}, fmt.Errorf("ledger: identifier must be 1-32 uppercase ASCII characters: %q", value)
	}
	var id ID
	copy(id[:], value)
	return id, nil
}
func MustID(value string) ID {
	id, err := NewID(value)
	if err != nil {
		panic(err)
	}
	return id
}
func (id ID) String() string {
	end := len(id)
	for end > 0 && id[end-1] == 0 {
		end--
	}
	return string(id[:end])
}
func (a Address) String() string         { return "0x" + hex.EncodeToString(a[:]) }
func (h TransactionHash) String() string { return "0x" + hex.EncodeToString(h[:]) }

type OrganizationType uint8

const (
	OrganizationTypeHospital       OrganizationType = 1
	OrganizationTypePayer          OrganizationType = 2
	OrganizationTypeStaffingAgency OrganizationType = 3
)

type AuthorityType uint8

const AuthorityTypeExclusion AuthorityType = 1

type EventType uint8

const EventTypeExclusion EventType = 1

type AssertionKind uint8

const (
	AssertionOriginal      AssertionKind = 1
	AssertionCorrection    AssertionKind = 2
	AssertionSupersession  AssertionKind = 3
	AssertionReinstatement AssertionKind = 4
)

type AuthorityEffect uint8

const (
	EffectExclusionActive AuthorityEffect = 1
	EffectExclusionLifted AuthorityEffect = 2
)

type ResponseState uint8

const (
	ResponseReceived        ResponseState = 1
	ResponseUnderReview     ResponseState = 2
	ResponseDecided         ResponseState = 3
	ResponseActionCompleted ResponseState = 4
)

type Organization struct {
	ID                  ID
	Type                OrganizationType
	Active              bool
	RegisteredAt        uint64
	AuthorizedSubmitter Address
}
type Authority struct {
	ID                  ID
	Type                AuthorityType
	Active              bool
	RegisteredAt        uint64
	AuthorizedSubmitter Address
}
type AuthorityAssertion struct {
	EventID, EventSeriesID, PreviousEventID, TargetEventID, AuthorityID ID
	EventCommitment                                                     Commitment
	EventType                                                           EventType
	AssertionKind                                                       AssertionKind
	AuthorityEffect                                                     AuthorityEffect
	EffectiveTime                                                       int64
	RecordedAt                                                          uint64
}
type ResponseVersion struct {
	ResponseID, ResponseVersionID, PreviousResponseVersionID, EventID, OrganizationID ID
	State                                                                             ResponseState
	ReceiptTimestamp                                                                  int64
	PolicyVersionHash, DecisionCommitment, ActionCommitment                           Commitment
	RecordedAt                                                                        uint64
}
type Transaction struct {
	Hash        TransactionHash
	BlockNumber BlockNumber
	BlockHash   TransactionHash
	ConfirmedAt time.Time
}

type ReceiptStatus uint8

const (
	ReceiptUnknown ReceiptStatus = iota
	ReceiptSuccessful
	ReceiptReverted
)

type Receipt struct {
	Transaction
	Status ReceiptStatus
}

var (
	ErrInvalidConfiguration = errors.New("ledger: invalid configuration")
	ErrChainIDMismatch      = errors.New("ledger: chain ID mismatch")
	ErrTransport            = errors.New("ledger: RPC transport failure")
	ErrSigning              = errors.New("ledger: transaction signing failure")
	ErrSubmission           = errors.New("ledger: transaction submission failure")
	ErrContractRevert       = errors.New("ledger: contract reverted")
	ErrReceiptTimeout       = errors.New("ledger: receipt confirmation timeout")
)

type Interface interface {
	ChainID(context.Context) (*big.Int, error)
	RegisterOrganization(context.Context, ID, OrganizationType, Address) (Transaction, error)
	RegisterAuthority(context.Context, ID, AuthorityType, Address) (Transaction, error)
	RecordAuthorityAssertion(context.Context, AuthorityAssertion) (Transaction, error)
	AppendResponseVersion(context.Context, ResponseVersion) (Transaction, error)
	GetOrganization(context.Context, ID) (Organization, error)
	GetAuthority(context.Context, ID) (Authority, error)
	GetAuthorityAssertion(context.Context, ID) (AuthorityAssertion, error)
	GetCurrentAuthorityHead(context.Context, ID) (ID, error)
	GetResponseVersion(context.Context, ID) (ResponseVersion, error)
	GetLatestResponse(context.Context, ID, ID) (ResponseVersion, error)
	TransactionReceipt(context.Context, TransactionHash) (Receipt, error)
	Close()
}
