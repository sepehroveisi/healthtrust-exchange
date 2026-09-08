package ledger

import "context"

type ChainRecordStatus uint8

const (
	ChainRecordAbsent ChainRecordStatus = iota
	ChainRecordPresent
)

type AuthorityAssertionLookup struct {
	Status    ChainRecordStatus
	Assertion AuthorityAssertion
	Receipt   Receipt
}

type ResponseVersionLookup struct {
	Status   ChainRecordStatus
	Response ResponseVersion
	Receipt  Receipt
}

type ReconciliationInterface interface {
	SubmissionInterface
	LookupAuthorityAssertion(context.Context, ID) (AuthorityAssertionLookup, error)
	LookupResponseVersion(context.Context, ID) (ResponseVersionLookup, error)
}
