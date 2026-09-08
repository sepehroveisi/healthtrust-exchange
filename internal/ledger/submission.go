package ledger

import "context"

type SubmissionInterface interface {
	BroadcastAuthorityAssertion(context.Context, AuthorityAssertion) (TransactionHash, error)
	BroadcastResponseVersion(context.Context, ResponseVersion) (TransactionHash, error)
	ObserveReceipt(context.Context, TransactionHash) (Receipt, error)
	WaitReceipt(context.Context, TransactionHash) (Receipt, error)
}
