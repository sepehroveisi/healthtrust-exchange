package tamperdemo

import (
	"context"
	"errors"
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	ledgerHash = "0xaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	localHash  = "0xbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

type fakeRepository struct {
	assertion  authorityledger.AuthorityAssertion
	series     authorityledger.EventSeries
	stream     authorityledger.ResponseStream
	history    []authorityledger.ResponseVersion
	policy     authorityledger.Policy
	submission authorityledger.Submission
	err        error
}

func validRepository() *fakeRepository {
	block := int64(7)
	return &fakeRepository{
		assertion:  authorityledger.AuthorityAssertion{ID: FixtureEventID, SeriesID: "SERIES-PHASE8", EventType: "EXCLUSION", Kind: "ORIGINAL"},
		series:     authorityledger.EventSeries{ID: "SERIES-PHASE8", AuthorityID: FixtureAuthorityID, SubjectID: FixtureSubjectID},
		stream:     authorityledger.ResponseStream{ID: "P8-HOSP", EventID: FixtureEventID, OrganizationID: FixtureOrganization},
		history:    []authorityledger.ResponseVersion{{ID: "P8-HOSP-V4", ResponseID: "P8-HOSP", EventID: FixtureEventID, OrganizationID: FixtureOrganization, State: "ACTION_COMPLETED", PolicyVersionID: stringPointer("POL-P8-HOSP-V1")}},
		policy:     authorityledger.Policy{ID: "POL-P8-HOSP", VersionID: "POL-P8-HOSP-V1", OrganizationID: FixtureOrganization},
		submission: authorityledger.Submission{State: "CONFIRMED", TransactionHash: make([]byte, 32), BlockNumber: &block, BlockHash: make([]byte, 32)},
	}
}

func stringPointer(value string) *string { return &value }

func (r *fakeRepository) GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error) {
	return r.assertion, r.err
}
func (r *fakeRepository) GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error) {
	return r.series, r.err
}
func (r *fakeRepository) GetResponseStreamForEvent(context.Context, authorityledger.AssertionID, authorityledger.OrganizationID) (authorityledger.ResponseStream, error) {
	return r.stream, r.err
}
func (r *fakeRepository) ResponseHistory(context.Context, authorityledger.ResponseID) ([]authorityledger.ResponseVersion, error) {
	return r.history, r.err
}
func (r *fakeRepository) GetPolicy(context.Context, string) (authorityledger.Policy, error) {
	return r.policy, r.err
}
func (r *fakeRepository) GetSubmissionByDomain(context.Context, string, string) (authorityledger.Submission, error) {
	return r.submission, r.err
}

type fakeExecutor struct {
	calls int
	tag   pgconn.CommandTag
	err   error
}

func (e *fakeExecutor) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	e.calls++
	return e.tag, e.err
}

type verificationReply struct {
	result integrityverification.Result
	err    error
}

type fakeVerifier struct {
	authority []verificationReply
	response  []verificationReply
	aCalls    int
	rCalls    int
}

func (v *fakeVerifier) VerifyAuthorityEvent(context.Context, string) (integrityverification.Result, error) {
	index := v.aCalls
	v.aCalls++
	if index >= len(v.authority) {
		index = len(v.authority) - 1
	}
	return v.authority[index].result, v.authority[index].err
}
func (v *fakeVerifier) VerifyResponse(context.Context, string, string) (integrityverification.Result, error) {
	index := v.rCalls
	v.rCalls++
	if index >= len(v.response) {
		index = len(v.response) - 1
	}
	return v.response[index].result, v.response[index].err
}

func authorityResult(status integrityverification.VerificationStatus, local string) integrityverification.Result {
	return integrityverification.Result{Status: status, ObjectType: "AUTHORITY_EVENT", ObjectID: FixtureEventID, Checks: []integrityverification.Check{{Type: integrityverification.CheckCanonicalCommitment, Status: status, ObjectID: FixtureEventID, Expected: ledgerHash, Observed: local}}}
}

func responseResult(status integrityverification.VerificationStatus, local string) integrityverification.Result {
	return integrityverification.Result{Status: status, ObjectType: "ORGANIZATION_RESPONSE", ObjectID: "P8-HOSP", Checks: []integrityverification.Check{{Type: integrityverification.CheckCanonicalCommitment, Status: status, ObjectID: "P8-HOSP-V4:policy", Expected: local, Observed: ledgerHash}}}
}

func successfulVerifier() *fakeVerifier {
	return &fakeVerifier{
		authority: []verificationReply{{result: authorityResult(integrityverification.StatusVerified, ledgerHash)}, {result: authorityResult(integrityverification.StatusFailed, localHash)}},
		response:  []verificationReply{{result: responseResult(integrityverification.StatusVerified, ledgerHash)}, {result: responseResult(integrityverification.StatusFailed, localHash)}},
	}
}

func TestEnabledFromEnvironmentRequiresExactOptIn(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"TRUE", false}, {"1", false}, {"true ", false}, {"true", true}} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(EnvironmentVariable, test.value)
			if got := EnabledFromEnvironment(); got != test.want {
				t.Fatalf("EnabledFromEnvironment()=%v want=%v", got, test.want)
			}
		})
	}
}

func TestParseTargetAllowlist(t *testing.T) {
	for _, value := range []string{string(TargetAuthorityEvent), string(TargetHospitalAPolicy)} {
		if target, err := ParseTarget(value); err != nil || string(target) != value {
			t.Fatalf("ParseTarget(%q)=%q,%v", value, target, err)
		}
	}
	for _, value := range []string{"", "PAYER_B_POLICY", "policies", "policy_text", "SELECT * FROM policies", `{"column":"policy_text"}`} {
		if _, err := ParseTarget(value); !errors.Is(err, ErrUnsupportedTarget) {
			t.Fatalf("ParseTarget(%q) error=%v", value, err)
		}
	}
}

func TestSuccessfulControlledMutations(t *testing.T) {
	for _, target := range []Target{TargetAuthorityEvent, TargetHospitalAPolicy} {
		t.Run(string(target), func(t *testing.T) {
			repo := validRepository()
			executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
			result, err := newService(true, repo, executor, successfulVerifier()).Demonstrate(context.Background(), target)
			if err != nil {
				t.Fatal(err)
			}
			if !result.DemoOnly || result.Warning != DemoWarning || result.Before.Status != integrityverification.StatusVerified || result.After.Status != integrityverification.StatusFailed || result.Before.LocalCommitment != ledgerHash || result.After.LocalCommitment != localHash || result.After.LedgerCommitment != ledgerHash || !result.LedgerUnchanged || executor.calls != 1 {
				t.Fatalf("unexpected result %+v calls=%d", result, executor.calls)
			}
		})
	}
}

func TestDisabledFailsClosedBeforeAnyDependency(t *testing.T) {
	executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
	_, err := newService(false, nil, executor, nil).Demonstrate(context.Background(), TargetAuthorityEvent)
	if !errors.Is(err, ErrDemoTamperDisabled) || executor.calls != 0 {
		t.Fatalf("error=%v calls=%d", err, executor.calls)
	}
}

func TestPreconditionAndFixtureFailuresDoNotMutate(t *testing.T) {
	tests := []struct {
		name   string
		target Target
		setup  func(*fakeRepository, *fakeVerifier)
		want   error
	}{
		{"missing fixture", TargetAuthorityEvent, func(r *fakeRepository, _ *fakeVerifier) { r.err = pgx.ErrNoRows }, ErrSyntheticFixtureMissing},
		{"wrong authority", TargetAuthorityEvent, func(r *fakeRepository, _ *fakeVerifier) { r.series.AuthorityID = "OTHER" }, ErrSyntheticFixtureMismatch},
		{"wrong subject", TargetAuthorityEvent, func(r *fakeRepository, _ *fakeVerifier) { r.series.SubjectID = "OTHER" }, ErrSyntheticFixtureMismatch},
		{"wrong event type", TargetAuthorityEvent, func(r *fakeRepository, _ *fakeVerifier) { r.assertion.EventType = "OTHER" }, ErrSyntheticFixtureMismatch},
		{"wrong organization", TargetHospitalAPolicy, func(r *fakeRepository, _ *fakeVerifier) { r.policy.OrganizationID = "PAYER-B" }, ErrSyntheticFixtureMismatch},
		{"not completed", TargetHospitalAPolicy, func(r *fakeRepository, _ *fakeVerifier) { r.history[0].State = "DECIDED" }, ErrSyntheticFixtureMismatch},
		{"not confirmed", TargetAuthorityEvent, func(r *fakeRepository, _ *fakeVerifier) { r.submission.State = "SUBMITTED" }, ErrPrecondition},
		{"already failed", TargetAuthorityEvent, func(_ *fakeRepository, v *fakeVerifier) {
			v.authority = []verificationReply{{result: authorityResult(integrityverification.StatusFailed, localHash)}}
		}, ErrPrecondition},
		{"indeterminate", TargetAuthorityEvent, func(_ *fakeRepository, v *fakeVerifier) {
			v.authority = []verificationReply{{result: authorityResult(integrityverification.StatusIndeterminate, ""), err: integrityverification.ErrUnavailable}}
		}, ErrVerificationUnavailable},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := validRepository()
			verifier := successfulVerifier()
			test.setup(repo, verifier)
			executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
			_, err := newService(true, repo, executor, verifier).Demonstrate(context.Background(), test.target)
			if !errors.Is(err, test.want) || executor.calls != 0 {
				t.Fatalf("error=%v want=%v calls=%d", err, test.want, executor.calls)
			}
		})
	}
}

func TestMutationAndPostconditionFailures(t *testing.T) {
	t.Run("database error", func(t *testing.T) {
		executor := &fakeExecutor{err: errors.New("write refused")}
		_, err := newService(true, validRepository(), executor, successfulVerifier()).Demonstrate(context.Background(), TargetAuthorityEvent)
		if !errors.Is(err, ErrMutationFailed) || executor.calls != 1 {
			t.Fatalf("error=%v calls=%d", err, executor.calls)
		}
	})
	t.Run("wrong row count", func(t *testing.T) {
		executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 0")}
		_, err := newService(true, validRepository(), executor, successfulVerifier()).Demonstrate(context.Background(), TargetAuthorityEvent)
		if !errors.Is(err, ErrMutationFailed) {
			t.Fatal(err)
		}
	})
	t.Run("unexpected verified", func(t *testing.T) {
		verifier := successfulVerifier()
		verifier.authority[1] = verificationReply{result: authorityResult(integrityverification.StatusVerified, ledgerHash)}
		_, err := newService(true, validRepository(), &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}, verifier).Demonstrate(context.Background(), TargetAuthorityEvent)
		if !errors.Is(err, ErrPostcondition) {
			t.Fatal(err)
		}
	})
	t.Run("post mutation unavailable", func(t *testing.T) {
		verifier := successfulVerifier()
		verifier.authority[1] = verificationReply{result: authorityResult(integrityverification.StatusIndeterminate, ""), err: integrityverification.ErrUnavailable}
		_, err := newService(true, validRepository(), &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}, verifier).Demonstrate(context.Background(), TargetAuthorityEvent)
		if !errors.Is(err, ErrPostcondition) {
			t.Fatal(err)
		}
	})
}

func TestRepeatedTamperIsRejectedBeforeSecondWrite(t *testing.T) {
	executor := &fakeExecutor{tag: pgconn.NewCommandTag("UPDATE 1")}
	verifier := successfulVerifier()
	service := newService(true, validRepository(), executor, verifier)
	if _, err := service.Demonstrate(context.Background(), TargetAuthorityEvent); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Demonstrate(context.Background(), TargetAuthorityEvent); !errors.Is(err, ErrPrecondition) {
		t.Fatalf("second error=%v", err)
	}
	if executor.calls != 1 {
		t.Fatalf("writes=%d", executor.calls)
	}
}
