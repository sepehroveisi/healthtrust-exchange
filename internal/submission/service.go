package submission

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

const (
	OperationAuthorityAssertion = "AUTHORITY_ASSERTION"
	OperationResponseVersion    = "RESPONSE_VERSION"
)

var (
	ErrInvalidSubmission        = errors.New("submission: invalid operation")
	ErrUnknownSigner            = errors.New("submission: unknown signer context")
	ErrPostBroadcastPersistence = errors.New("submission: transaction broadcast but state persistence failed")
	ErrPostReceiptPersistence   = errors.New("submission: receipt observed but state persistence failed")
)

type Repository interface {
	GetSubmission(context.Context, authorityledger.OperationID) (authorityledger.Submission, error)
	GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error)
	GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error)
	GetResponse(context.Context, authorityledger.ResponseVersionID) (authorityledger.ResponseVersion, error)
	GetPolicy(context.Context, string) (authorityledger.Policy, error)
	GetDecision(context.Context, string) (authorityledger.Decision, error)
	GetAction(context.Context, string) (authorityledger.Action, error)
	MarkSubmitted(context.Context, authorityledger.OperationID, []byte, time.Time) error
	RecordBroadcastAmbiguous(context.Context, authorityledger.OperationID, string, time.Time) error
	MarkBroadcastFailed(context.Context, authorityledger.OperationID, string, time.Time) error
	MarkReceiptPending(context.Context, authorityledger.OperationID, string) error
	MarkConfirmed(context.Context, authorityledger.OperationID, []byte, int64, []byte) error
	MarkReceiptFailed(context.Context, authorityledger.OperationID, []byte, string) error
}

type Service struct {
	repository Repository
	signers    map[string]ledger.SubmissionInterface
	now        func() time.Time
}

func New(repository Repository, signers map[string]ledger.SubmissionInterface) *Service {
	return &Service{repository: repository, signers: signers, now: time.Now}
}

func (s *Service) Submit(ctx context.Context, operationID authorityledger.OperationID) error {
	sub, err := s.repository.GetSubmission(ctx, operationID)
	if err != nil {
		return err
	}
	if sub.State != "LOCAL_PENDING" {
		return fmt.Errorf("%w: state %s", ErrInvalidSubmission, sub.State)
	}
	signer, ok := s.signers[sub.SignerContext]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownSigner, sub.SignerContext)
	}

	var hash ledger.TransactionHash
	switch sub.OperationType {
	case OperationAuthorityAssertion:
		assertion, loadErr := s.repository.GetAssertion(ctx, authorityledger.AssertionID(sub.DomainVersionID))
		if loadErr != nil {
			return loadErr
		}
		if assertion.Commitment != sub.IntendedCommitment {
			return fmt.Errorf("%w: commitment mismatch", ErrInvalidSubmission)
		}
		series, loadErr := s.repository.GetEventSeries(ctx, assertion.SeriesID)
		if loadErr != nil {
			return loadErr
		}
		value, convertErr := assertionValue(assertion, series.AuthorityID)
		if convertErr != nil {
			return convertErr
		}
		hash, err = signer.BroadcastAuthorityAssertion(ctx, value)
	case OperationResponseVersion:
		response, loadErr := s.repository.GetResponse(ctx, authorityledger.ResponseVersionID(sub.DomainVersionID))
		if loadErr != nil {
			return loadErr
		}
		if response.Commitment != sub.IntendedCommitment {
			return fmt.Errorf("%w: commitment mismatch", ErrInvalidSubmission)
		}
		value, convertErr := s.responseValue(ctx, response)
		if convertErr != nil {
			return convertErr
		}
		hash, err = signer.BroadcastResponseVersion(ctx, value)
	default:
		return fmt.Errorf("%w: type %s", ErrInvalidSubmission, sub.OperationType)
	}
	if err != nil {
		return s.recordBroadcastError(ctx, sub.OperationID, err)
	}
	if err = s.repository.MarkSubmitted(ctx, sub.OperationID, hash[:], s.now().UTC()); err != nil {
		return fmt.Errorf("%w: %v", ErrPostBroadcastPersistence, err)
	}
	receipt, err := signer.WaitReceipt(ctx, hash)
	if err != nil {
		class := "RECEIPT_ERROR"
		if errors.Is(err, ledger.ErrReceiptTimeout) {
			class = "RECEIPT_TIMEOUT"
		}
		if errors.Is(err, ledger.ErrTransport) {
			class = "RECEIPT_TRANSPORT"
		}
		if persistErr := s.repository.MarkReceiptPending(ctx, sub.OperationID, class); persistErr != nil {
			return fmt.Errorf("%w: %v", ErrPostReceiptPersistence, persistErr)
		}
		return err
	}
	if receipt.Hash != hash {
		return fmt.Errorf("%w: receipt transaction mismatch", ErrInvalidSubmission)
	}
	if receipt.Status == ledger.ReceiptReverted {
		if err = s.repository.MarkReceiptFailed(ctx, sub.OperationID, hash[:], "RECEIPT_REVERTED"); err != nil {
			return fmt.Errorf("%w: %v", ErrPostReceiptPersistence, err)
		}
		return ledger.ErrContractRevert
	}
	if receipt.Status != ledger.ReceiptSuccessful {
		return fmt.Errorf("%w: unknown receipt status", ErrInvalidSubmission)
	}
	if receipt.BlockNumber > ledger.BlockNumber(^uint64(0)>>1) {
		return fmt.Errorf("%w: block number overflow", ErrInvalidSubmission)
	}
	if err = s.repository.MarkConfirmed(ctx, sub.OperationID, hash[:], int64(receipt.BlockNumber), receipt.BlockHash[:]); err != nil {
		return fmt.Errorf("%w: %v", ErrPostReceiptPersistence, err)
	}
	return nil
}

func (s *Service) recordBroadcastError(ctx context.Context, id authorityledger.OperationID, err error) error {
	class := "BROADCAST_ERROR"
	definite := errors.Is(err, ledger.ErrContractRevert) || errors.Is(err, ledger.ErrSigning) || errors.Is(err, ledger.ErrInvalidConfiguration)
	if errors.Is(err, ledger.ErrContractRevert) {
		class = "BROADCAST_REVERTED"
	}
	if errors.Is(err, ledger.ErrSigning) {
		class = "SIGNING"
	}
	if errors.Is(err, ledger.ErrInvalidConfiguration) {
		class = "CONFIGURATION"
	}
	var persistErr error
	if definite {
		persistErr = s.repository.MarkBroadcastFailed(ctx, id, class, s.now().UTC())
	} else {
		persistErr = s.repository.RecordBroadcastAmbiguous(ctx, id, class, s.now().UTC())
	}
	if persistErr != nil {
		return fmt.Errorf("%w: %v", ErrPostBroadcastPersistence, persistErr)
	}
	return err
}

func assertionValue(v authorityledger.AuthorityAssertion, authorityID authorityledger.AuthorityID) (ledger.AuthorityAssertion, error) {
	event, err := ledger.NewID(string(v.ID))
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	series, err := ledger.NewID(string(v.SeriesID))
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	authority, err := ledger.NewID(string(authorityID))
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	previous, err := optionalID(v.PreviousID)
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	target, err := optionalID(v.TargetID)
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	eventType := map[string]ledger.EventType{"EXCLUSION": ledger.EventTypeExclusion}[v.EventType]
	kind := map[string]ledger.AssertionKind{"ORIGINAL": ledger.AssertionOriginal, "CORRECTION": ledger.AssertionCorrection, "SUPERSESSION": ledger.AssertionSupersession, "REINSTATEMENT": ledger.AssertionReinstatement}[v.Kind]
	effect := map[string]ledger.AuthorityEffect{"EXCLUSION_ACTIVE": ledger.EffectExclusionActive, "EXCLUSION_LIFTED": ledger.EffectExclusionLifted}[v.Effect]
	if eventType == 0 || kind == 0 || effect == 0 {
		return ledger.AuthorityAssertion{}, fmt.Errorf("%w: assertion enum", ErrInvalidSubmission)
	}
	return ledger.AuthorityAssertion{EventID: event, EventSeriesID: series, PreviousEventID: previous, TargetEventID: target, AuthorityID: authority, EventCommitment: ledger.Commitment(v.Commitment), EventType: eventType, AssertionKind: kind, AuthorityEffect: effect, EffectiveTime: v.EffectiveUnixNano}, nil
}

func optionalID[T ~string](value *T) (ledger.ID, error) {
	if value == nil {
		return ledger.ID{}, nil
	}
	return ledger.NewID(string(*value))
}

func (s *Service) responseValue(ctx context.Context, v authorityledger.ResponseVersion) (ledger.ResponseVersion, error) {
	response, err := ledger.NewID(string(v.ResponseID))
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	version, err := ledger.NewID(string(v.ID))
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	event, err := ledger.NewID(string(v.EventID))
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	organization, err := ledger.NewID(string(v.OrganizationID))
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	previous, err := optionalID(v.PreviousID)
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	state := map[string]ledger.ResponseState{"RECEIVED": ledger.ResponseReceived, "UNDER_REVIEW": ledger.ResponseUnderReview, "DECIDED": ledger.ResponseDecided, "ACTION_COMPLETED": ledger.ResponseActionCompleted}[v.State]
	if state == 0 {
		return ledger.ResponseVersion{}, fmt.Errorf("%w: response state", ErrInvalidSubmission)
	}
	var policy, decision, action ledger.Commitment
	if v.PolicyVersionID != nil {
		p, e := s.repository.GetPolicy(ctx, *v.PolicyVersionID)
		if e != nil {
			return ledger.ResponseVersion{}, e
		}
		policy = ledger.Commitment(p.Commitment)
	}
	if v.DecisionID != nil {
		d, e := s.repository.GetDecision(ctx, *v.DecisionID)
		if e != nil {
			return ledger.ResponseVersion{}, e
		}
		decision = ledger.Commitment(d.Commitment)
	}
	if v.ActionID != nil {
		a, e := s.repository.GetAction(ctx, *v.ActionID)
		if e != nil {
			return ledger.ResponseVersion{}, e
		}
		action = ledger.Commitment(a.Commitment)
	}
	return ledger.ResponseVersion{ResponseID: response, ResponseVersionID: version, PreviousResponseVersionID: previous, EventID: event, OrganizationID: organization, State: state, ReceiptTimestamp: v.ReceiptUnixNano, PolicyVersionHash: policy, DecisionCommitment: decision, ActionCommitment: action}, nil
}
