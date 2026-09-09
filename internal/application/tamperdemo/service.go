package tamperdemo

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	mutatedAuthoritySource = "phase8-demo-mutated-authority-source"
	mutatedHospitalPolicy  = "phase8-demo-mutated-hospital-policy"
)

type Repository interface {
	GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error)
	GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error)
	GetResponseStreamForEvent(context.Context, authorityledger.AssertionID, authorityledger.OrganizationID) (authorityledger.ResponseStream, error)
	ResponseHistory(context.Context, authorityledger.ResponseID) ([]authorityledger.ResponseVersion, error)
	GetPolicy(context.Context, string) (authorityledger.Policy, error)
	GetSubmissionByDomain(context.Context, string, string) (authorityledger.Submission, error)
}

type Executor interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

type Verifier interface {
	VerifyAuthorityEvent(context.Context, string) (integrityverification.Result, error)
	VerifyResponse(context.Context, string, string) (integrityverification.Result, error)
}

type Service struct {
	enabled  bool
	repo     Repository
	executor Executor
	verifier Verifier
}

func New(repo Repository, executor Executor, verifier Verifier) *Service {
	return newService(EnabledFromEnvironment(), repo, executor, verifier)
}

func newService(enabled bool, repo Repository, executor Executor, verifier Verifier) *Service {
	return &Service{enabled: enabled, repo: repo, executor: executor, verifier: verifier}
}

func (s *Service) Demonstrate(ctx context.Context, target Target) (Result, error) {
	if !s.enabled {
		return Result{}, ErrDemoTamperDisabled
	}
	if target != TargetAuthorityEvent && target != TargetHospitalAPolicy {
		return Result{}, ErrUnsupportedTarget
	}

	meta, err := s.validateFixture(ctx, target)
	if err != nil {
		return Result{}, err
	}
	before, err := s.verify(ctx, target, meta)
	if err != nil {
		return Result{}, err
	}
	if before.Status != integrityverification.StatusVerified || before.LocalCommitment == "" || before.LocalCommitment != before.LedgerCommitment {
		return Result{}, ErrPrecondition
	}

	result := Result{
		DemoOnly:       true,
		Warning:        DemoWarning,
		Target:         target,
		EventID:        FixtureEventID,
		OrganizationID: meta.organizationID,
		MutationType:   "CONTROLLED_POST_COMMITMENT_MUTATION",
		ChangedField:   meta.changedField,
		Before:         before,
	}
	if err = s.mutate(ctx, target, meta); err != nil {
		return result, fmt.Errorf("%w: %v", ErrMutationFailed, err)
	}

	result.After, err = s.verify(ctx, target, meta)
	if err != nil {
		if errors.Is(err, ErrVerificationUnavailable) {
			return result, fmt.Errorf("%w: %v", ErrPostcondition, err)
		}
		return result, err
	}
	result.LedgerUnchanged = result.Before.LedgerCommitment == result.After.LedgerCommitment
	if result.After.Status != integrityverification.StatusFailed ||
		result.After.LocalCommitment == result.After.LedgerCommitment ||
		!result.LedgerUnchanged {
		return result, ErrPostcondition
	}
	return result, nil
}

type fixtureMetadata struct {
	versionID, policyVersionID, organizationID, changedField string
}

func (s *Service) validateFixture(ctx context.Context, target Target) (fixtureMetadata, error) {
	assertion, err := s.repo.GetAssertion(ctx, authorityledger.AssertionID(FixtureEventID))
	if err != nil {
		return fixtureMetadata{}, fixtureError(err)
	}
	series, err := s.repo.GetEventSeries(ctx, assertion.SeriesID)
	if err != nil {
		return fixtureMetadata{}, fixtureError(err)
	}
	if assertion.ID != FixtureEventID || series.ID != assertion.SeriesID ||
		series.AuthorityID != FixtureAuthorityID || series.SubjectID != FixtureSubjectID ||
		assertion.EventType != "EXCLUSION" || assertion.Kind != "ORIGINAL" {
		return fixtureMetadata{}, ErrSyntheticFixtureMismatch
	}
	if err = confirmed(s.repo, ctx, submission.OperationAuthorityAssertion, FixtureEventID); err != nil {
		return fixtureMetadata{}, err
	}
	if target == TargetAuthorityEvent {
		return fixtureMetadata{changedField: "authority_event.source_document"}, nil
	}

	stream, err := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(FixtureEventID), authorityledger.OrganizationID(FixtureOrganization))
	if err != nil {
		return fixtureMetadata{}, fixtureError(err)
	}
	if stream.EventID != FixtureEventID || stream.OrganizationID != FixtureOrganization {
		return fixtureMetadata{}, ErrSyntheticFixtureMismatch
	}
	history, err := s.repo.ResponseHistory(ctx, stream.ID)
	if err != nil || len(history) == 0 {
		return fixtureMetadata{}, fixtureError(err)
	}
	latest := history[len(history)-1]
	if latest.EventID != FixtureEventID || latest.OrganizationID != FixtureOrganization ||
		latest.State != "ACTION_COMPLETED" || latest.PolicyVersionID == nil {
		return fixtureMetadata{}, ErrSyntheticFixtureMismatch
	}
	policy, err := s.repo.GetPolicy(ctx, *latest.PolicyVersionID)
	if err != nil {
		return fixtureMetadata{}, fixtureError(err)
	}
	if policy.OrganizationID != FixtureOrganization || policy.VersionID != *latest.PolicyVersionID {
		return fixtureMetadata{}, ErrSyntheticFixtureMismatch
	}
	if err = confirmed(s.repo, ctx, submission.OperationResponseVersion, string(latest.ID)); err != nil {
		return fixtureMetadata{}, err
	}
	return fixtureMetadata{versionID: string(latest.ID), policyVersionID: policy.VersionID, organizationID: FixtureOrganization, changedField: "policy.policy_text"}, nil
}

func confirmed(repo Repository, ctx context.Context, kind, domainID string) error {
	value, err := repo.GetSubmissionByDomain(ctx, kind, domainID)
	if err != nil {
		return fixtureError(err)
	}
	if value.State != "CONFIRMED" || len(value.TransactionHash) != 32 || value.BlockNumber == nil || len(value.BlockHash) != 32 {
		return ErrPrecondition
	}
	return nil
}

func fixtureError(err error) error {
	if err == nil || errors.Is(err, pgx.ErrNoRows) {
		return ErrSyntheticFixtureMissing
	}
	return err
}

func (s *Service) verify(ctx context.Context, target Target, meta fixtureMetadata) (Snapshot, error) {
	var result integrityverification.Result
	var err error
	if target == TargetAuthorityEvent {
		result, err = s.verifier.VerifyAuthorityEvent(ctx, FixtureEventID)
	} else {
		result, err = s.verifier.VerifyResponse(ctx, FixtureEventID, FixtureOrganization)
	}
	if errors.Is(err, integrityverification.ErrUnavailable) || result.Status == integrityverification.StatusIndeterminate {
		return Snapshot{Status: result.Status, Verification: result}, ErrVerificationUnavailable
	}
	if errors.Is(err, integrityverification.ErrNotFound) {
		return Snapshot{}, ErrSyntheticFixtureMissing
	}
	if err != nil {
		return Snapshot{}, err
	}
	local, committed := commitments(target, meta, result)
	return Snapshot{Status: result.Status, LocalCommitment: local, LedgerCommitment: committed, Verification: result}, nil
}

func commitments(target Target, meta fixtureMetadata, result integrityverification.Result) (string, string) {
	objectID := FixtureEventID
	if target == TargetHospitalAPolicy {
		objectID = meta.versionID + ":policy"
	}
	var local, committed string
	for _, check := range result.Checks {
		if check.Type != integrityverification.CheckCanonicalCommitment || check.ObjectID != objectID {
			continue
		}
		if target == TargetAuthorityEvent {
			committed, local = check.Expected, check.Observed
		} else {
			local, committed = check.Expected, check.Observed
		}
	}
	return local, committed
}

func (s *Service) mutate(ctx context.Context, target Target, meta fixtureMetadata) error {
	var tag pgconn.CommandTag
	var err error
	if target == TargetAuthorityEvent {
		tag, err = s.executor.Exec(ctx, `UPDATE authority_ledger.authority_assertions SET source_document=$1 WHERE id=$2 AND event_type='EXCLUSION' AND assertion_kind='ORIGINAL'`, []byte(mutatedAuthoritySource), FixtureEventID)
	} else {
		tag, err = s.executor.Exec(ctx, `UPDATE authority_ledger.policies SET policy_text=$1 WHERE version_id=$2 AND organization_id=$3`, mutatedHospitalPolicy, meta.policyVersionID, FixtureOrganization)
	}
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return fmt.Errorf("expected exactly one synthetic row, changed %d", tag.RowsAffected())
	}
	return nil
}

func ParseTarget(value string) (Target, error) {
	target := Target(strings.TrimSpace(value))
	if target != TargetAuthorityEvent && target != TargetHospitalAPolicy {
		return "", ErrUnsupportedTarget
	}
	return target, nil
}
