package submission

import (
	"context"
	"errors"
	"fmt"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

var (
	ErrChainUnavailable      = errors.New("reconciliation: chain unavailable")
	ErrChainStateAmbiguous   = errors.New("reconciliation: chain state ambiguous")
	ErrOnChainConflict       = errors.New("reconciliation: on-chain record conflicts with local intent")
	ErrReceiptPending        = errors.New("reconciliation: receipt pending")
	ErrExplicitFailedReceipt = errors.New("reconciliation: explicit failed receipt")
	ErrLocalPersistence      = errors.New("reconciliation: local persistence failure")
	ErrUnsupportedIdentifier = errors.New("reconciliation: unsupported on-chain identifier")
	ErrRetryNotPermitted     = errors.New("reconciliation: retry not permitted")
)

type ReconciliationRepository interface {
	Repository
	Unresolved(context.Context) ([]authorityledger.Submission, error)
	MarkRecoveredConfirmed(context.Context, authorityledger.OperationID, []byte, int64, []byte) error
	RecordReconciliationError(context.Context, authorityledger.OperationID, string) error
}

type Outcome string

const (
	OutcomeAlreadyConfirmed Outcome = "ALREADY_CONFIRMED"
	OutcomeSubmitted        Outcome = "SUBMITTED_AND_CONFIRMED"
	OutcomeRecovered        Outcome = "RECOVERED_CONFIRMED"
	OutcomeStillPending     Outcome = "STILL_PENDING"
	OutcomeFailed           Outcome = "SUBMISSION_FAILED"
)

type Result struct {
	OperationID authorityledger.OperationID
	Outcome     Outcome
	Err         error
}

type Reconciler struct {
	repository ReconciliationRepository
	ledgers    map[string]ledger.ReconciliationInterface
	submitter  *Service
}

func NewReconciler(repository ReconciliationRepository, ledgers map[string]ledger.ReconciliationInterface) *Reconciler {
	submissionLedgers := make(map[string]ledger.SubmissionInterface, len(ledgers))
	for key, value := range ledgers {
		submissionLedgers[key] = value
	}
	return &Reconciler{repository: repository, ledgers: ledgers, submitter: New(repository, submissionLedgers)}
}

func (r *Reconciler) ReconcileOne(ctx context.Context, operationID authorityledger.OperationID) (Outcome, error) {
	sub, err := r.repository.GetSubmission(ctx, operationID)
	if err != nil {
		return "", fmt.Errorf("%w: load submission", ErrInvalidSubmission)
	}
	if sub.State == "CONFIRMED" {
		return OutcomeAlreadyConfirmed, nil
	}
	chain, ok := r.ledgers[sub.SignerContext]
	if !ok {
		return "", fmt.Errorf("%w: signer context", ErrInvalidSubmission)
	}
	expected, err := r.expected(ctx, sub)
	if err != nil {
		return "", err
	}
	if sub.State == "SUBMITTED" {
		return r.reconcileSubmitted(ctx, sub, chain, expected)
	}
	lookup, err := lookupExpected(ctx, chain, expected)
	if err != nil {
		return "", r.recordLookupError(ctx, sub.OperationID, err)
	}
	if lookup.status == ledger.ChainRecordPresent {
		if !lookup.matches {
			return "", r.recordError(ctx, sub.OperationID, "CHAIN_CONFLICT", ErrOnChainConflict)
		}
		if err = r.repository.MarkRecoveredConfirmed(ctx, sub.OperationID, lookup.receipt.Hash[:], int64(lookup.receipt.BlockNumber), lookup.receipt.BlockHash[:]); err != nil {
			return "", fmt.Errorf("%w: recovered confirmation", ErrLocalPersistence)
		}
		return OutcomeRecovered, nil
	}
	if sub.State == "LOCAL_PENDING" {
		if err = r.submitter.Submit(ctx, sub.OperationID); err != nil {
			return "", err
		}
		return OutcomeSubmitted, nil
	}
	if sub.State == "SUBMISSION_FAILED" {
		return "", r.recordError(ctx, sub.OperationID, "RETRY_NOT_PERMITTED", ErrRetryNotPermitted)
	}
	return "", fmt.Errorf("%w: state %s", ErrInvalidSubmission, sub.State)
}

func (r *Reconciler) reconcileSubmitted(ctx context.Context, sub authorityledger.Submission, chain ledger.ReconciliationInterface, expected expectedRecord) (Outcome, error) {
	if len(sub.TransactionHash) != 32 {
		return "", fmt.Errorf("%w: transaction hash", ErrInvalidSubmission)
	}
	var hash ledger.TransactionHash
	copy(hash[:], sub.TransactionHash)
	receipt, err := chain.ObserveReceipt(ctx, hash)
	if err != nil {
		return "", r.recordError(ctx, sub.OperationID, "CHAIN_UNAVAILABLE", ErrChainUnavailable)
	}
	if receipt.Status == ledger.ReceiptUnknown {
		return OutcomeStillPending, r.recordError(ctx, sub.OperationID, "RECEIPT_PENDING", ErrReceiptPending)
	}
	if receipt.Hash != hash {
		return "", r.recordError(ctx, sub.OperationID, "CHAIN_AMBIGUOUS", ErrChainStateAmbiguous)
	}
	if receipt.Status == ledger.ReceiptReverted {
		if err = r.repository.MarkReceiptFailed(ctx, sub.OperationID, hash[:], "RECEIPT_REVERTED"); err != nil {
			return "", fmt.Errorf("%w: failed receipt", ErrLocalPersistence)
		}
		return OutcomeFailed, ErrExplicitFailedReceipt
	}
	lookup, err := lookupExpected(ctx, chain, expected)
	if err != nil {
		return "", r.recordLookupError(ctx, sub.OperationID, err)
	}
	if lookup.status != ledger.ChainRecordPresent {
		return "", r.recordError(ctx, sub.OperationID, "CHAIN_AMBIGUOUS", ErrChainStateAmbiguous)
	}
	if !lookup.matches {
		return "", r.recordError(ctx, sub.OperationID, "CHAIN_CONFLICT", ErrOnChainConflict)
	}
	if lookup.receipt.Hash != receipt.Hash || lookup.receipt.BlockHash != receipt.BlockHash || lookup.receipt.BlockNumber != receipt.BlockNumber {
		return "", r.recordError(ctx, sub.OperationID, "CHAIN_AMBIGUOUS", ErrChainStateAmbiguous)
	}
	if err = r.repository.MarkConfirmed(ctx, sub.OperationID, hash[:], int64(receipt.BlockNumber), receipt.BlockHash[:]); err != nil {
		return "", fmt.Errorf("%w: confirmation", ErrLocalPersistence)
	}
	return OutcomeRecovered, nil
}

func (r *Reconciler) ReconcileUnresolved(ctx context.Context, limit int) []Result {
	if limit <= 0 {
		return nil
	}
	items, err := r.repository.Unresolved(ctx)
	if err != nil {
		return []Result{{Err: err}}
	}
	if len(items) > limit {
		items = items[:limit]
	}
	out := make([]Result, 0, len(items))
	for _, item := range items {
		outcome, e := r.ReconcileOne(ctx, item.OperationID)
		out = append(out, Result{item.OperationID, outcome, e})
	}
	return out
}
func (r *Reconciler) recordError(ctx context.Context, id authorityledger.OperationID, class string, result error) error {
	if err := r.repository.RecordReconciliationError(ctx, id, class); err != nil {
		return fmt.Errorf("%w: classification", ErrLocalPersistence)
	}
	return result
}
func (r *Reconciler) recordLookupError(ctx context.Context, id authorityledger.OperationID, err error) error {
	if errors.Is(err, ledger.ErrChainStateAmbiguous) {
		return r.recordError(ctx, id, "CHAIN_AMBIGUOUS", ErrChainStateAmbiguous)
	}
	return r.recordError(ctx, id, "CHAIN_UNAVAILABLE", ErrChainUnavailable)
}
func classifyLookupError(err error) error {
	if errors.Is(err, ledger.ErrChainStateAmbiguous) {
		return ErrChainStateAmbiguous
	}
	return ErrChainUnavailable
}

type expectedRecord struct {
	operationType string
	assertion     ledger.AuthorityAssertion
	response      ledger.ResponseVersion
}
type lookupResult struct {
	status  ledger.ChainRecordStatus
	matches bool
	receipt ledger.Receipt
}

func (r *Reconciler) expected(ctx context.Context, sub authorityledger.Submission) (expectedRecord, error) {
	switch sub.OperationType {
	case OperationAuthorityAssertion:
		a, e := r.repository.GetAssertion(ctx, authorityledger.AssertionID(sub.DomainVersionID))
		if e != nil {
			return expectedRecord{}, fmt.Errorf("%w: assertion", ErrInvalidSubmission)
		}
		if a.Commitment != sub.IntendedCommitment {
			return expectedRecord{}, fmt.Errorf("%w: commitment", ErrInvalidSubmission)
		}
		series, e := r.repository.GetEventSeries(ctx, a.SeriesID)
		if e != nil {
			return expectedRecord{}, fmt.Errorf("%w: series", ErrInvalidSubmission)
		}
		v, e := assertionValue(a, series.AuthorityID)
		if e != nil {
			return expectedRecord{}, fmt.Errorf("%w: %v", ErrUnsupportedIdentifier, e)
		}
		return expectedRecord{operationType: sub.OperationType, assertion: v}, nil
	case OperationResponseVersion:
		v, e := r.repository.GetResponse(ctx, authorityledger.ResponseVersionID(sub.DomainVersionID))
		if e != nil {
			return expectedRecord{}, fmt.Errorf("%w: response", ErrInvalidSubmission)
		}
		if v.Commitment != sub.IntendedCommitment {
			return expectedRecord{}, fmt.Errorf("%w: commitment", ErrInvalidSubmission)
		}
		x, e := r.submitter.responseValue(ctx, v)
		if e != nil {
			return expectedRecord{}, fmt.Errorf("%w: %v", ErrUnsupportedIdentifier, e)
		}
		return expectedRecord{operationType: sub.OperationType, response: x}, nil
	default:
		return expectedRecord{}, fmt.Errorf("%w: operation type", ErrInvalidSubmission)
	}
}
func lookupExpected(ctx context.Context, chain ledger.ReconciliationInterface, expected expectedRecord) (lookupResult, error) {
	if expected.operationType == OperationAuthorityAssertion {
		x, e := chain.LookupAuthorityAssertion(ctx, expected.assertion.EventID)
		if e != nil {
			return lookupResult{}, e
		}
		return lookupResult{x.Status, x.Status == ledger.ChainRecordPresent && equalAssertion(x.Assertion, expected.assertion), x.Receipt}, nil
	}
	x, e := chain.LookupResponseVersion(ctx, expected.response.ResponseVersionID)
	if e != nil {
		return lookupResult{}, e
	}
	return lookupResult{x.Status, x.Status == ledger.ChainRecordPresent && equalResponse(x.Response, expected.response), x.Receipt}, nil
}
func equalAssertion(a, b ledger.AuthorityAssertion) bool {
	return a.EventID == b.EventID && a.EventSeriesID == b.EventSeriesID && a.PreviousEventID == b.PreviousEventID && a.TargetEventID == b.TargetEventID && a.AuthorityID == b.AuthorityID && a.EventCommitment == b.EventCommitment && a.EventType == b.EventType && a.AssertionKind == b.AssertionKind && a.AuthorityEffect == b.AuthorityEffect && a.EffectiveTime == b.EffectiveTime
}
func equalResponse(a, b ledger.ResponseVersion) bool {
	return a.ResponseID == b.ResponseID && a.ResponseVersionID == b.ResponseVersionID && a.PreviousResponseVersionID == b.PreviousResponseVersionID && a.EventID == b.EventID && a.OrganizationID == b.OrganizationID && a.State == b.State && a.ReceiptTimestamp == b.ReceiptTimestamp && a.PolicyVersionHash == b.PolicyVersionHash && a.DecisionCommitment == b.DecisionCommitment && a.ActionCommitment == b.ActionCommitment
}
