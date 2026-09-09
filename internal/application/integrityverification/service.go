package integrityverification

import (
	"context"
	"errors"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/jackc/pgx/v5"
)

type Repository interface {
	GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error)
	GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error)
	AssertionHistory(context.Context, authorityledger.SeriesID) ([]authorityledger.AuthorityAssertion, error)
	GetResponseStreamForEvent(context.Context, authorityledger.AssertionID, authorityledger.OrganizationID) (authorityledger.ResponseStream, error)
	ListResponseStreamsForEvent(context.Context, authorityledger.AssertionID) ([]authorityledger.ResponseStream, error)
	GetResponse(context.Context, authorityledger.ResponseVersionID) (authorityledger.ResponseVersion, error)
	ResponseHistory(context.Context, authorityledger.ResponseID) ([]authorityledger.ResponseVersion, error)
	GetPolicy(context.Context, string) (authorityledger.Policy, error)
	GetDecision(context.Context, string) (authorityledger.Decision, error)
	GetAction(context.Context, string) (authorityledger.Action, error)
	GetSubmissionByDomain(context.Context, string, string) (authorityledger.Submission, error)
}

type LedgerReader interface {
	GetAuthorityAssertion(context.Context, ledger.ID) (ledger.AuthorityAssertion, error)
	GetCurrentAuthorityHead(context.Context, ledger.ID) (ledger.ID, error)
	GetResponseVersion(context.Context, ledger.ID) (ledger.ResponseVersion, error)
	GetLatestResponse(context.Context, ledger.ID, ledger.ID) (ledger.ResponseVersion, error)
	TransactionReceipt(context.Context, ledger.TransactionHash) (ledger.Receipt, error)
}

type Service struct {
	repo   Repository
	ledger LedgerReader
}

func New(repo Repository, reader LedgerReader) *Service { return &Service{repo: repo, ledger: reader} }

func validID(value string) error {
	if _, err := ledger.NewID(value); err != nil {
		return ErrInvalid
	}
	return nil
}

func missing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (s *Service) VerifyEventBundle(ctx context.Context, eventID string) (BundleResult, error) {
	authority, authorityErr := s.VerifyAuthorityEvent(ctx, eventID)
	if errors.Is(authorityErr, ErrNotFound) || errors.Is(authorityErr, ErrInvalid) {
		return BundleResult{}, authorityErr
	}
	requiredOrganizations := []string{"HOSPITAL-A", "PAYER-B", "STAFFING-AGENCY-C"}
	bundle := BundleResult{EventID: eventID, Authority: authority, Responses: make([]OrganizationResult, 0, len(requiredOrganizations))}
	statuses := []VerificationStatus{authority.Status}
	unavailable := errors.Is(authorityErr, ErrUnavailable)
	for _, organizationID := range requiredOrganizations {
		result, verifyErr := s.VerifyResponse(ctx, eventID, organizationID)
		if errors.Is(verifyErr, ErrNotFound) {
			result = Result{Status: StatusFailed, ObjectType: "ORGANIZATION_RESPONSE", ObjectID: eventID + ":" + organizationID, Checks: []Check{failed(CheckIdentity, eventID+":"+organizationID, "response present", "missing", ClassMissingDomainObject)}}
			verifyErr = nil
		}
		if verifyErr != nil && !errors.Is(verifyErr, ErrUnavailable) {
			return BundleResult{}, verifyErr
		}
		unavailable = unavailable || errors.Is(verifyErr, ErrUnavailable)
		bundle.Responses = append(bundle.Responses, OrganizationResult{OrganizationID: organizationID, Result: result})
		statuses = append(statuses, result.Status)
	}
	bundle.Status = aggregate(statuses...)
	if unavailable && bundle.Status != StatusFailed {
		return bundle, ErrUnavailable
	}
	return bundle, nil
}
