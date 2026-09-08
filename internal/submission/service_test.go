package submission

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

type fakeRepo struct {
	sub       authorityledger.Submission
	assertion authorityledger.AuthorityAssertion
	series    authorityledger.EventSeries
	response  authorityledger.ResponseVersion
	policy    authorityledger.Policy
	decision  authorityledger.Decision
	action    authorityledger.Action
	events    []string
	fail      string
}

func (f *fakeRepo) GetSubmission(context.Context, authorityledger.OperationID) (authorityledger.Submission, error) {
	return f.sub, nil
}
func (f *fakeRepo) GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error) {
	return f.assertion, nil
}
func (f *fakeRepo) GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error) {
	return f.series, nil
}
func (f *fakeRepo) GetResponse(context.Context, authorityledger.ResponseVersionID) (authorityledger.ResponseVersion, error) {
	return f.response, nil
}
func (f *fakeRepo) GetPolicy(context.Context, string) (authorityledger.Policy, error) {
	return f.policy, nil
}
func (f *fakeRepo) GetDecision(context.Context, string) (authorityledger.Decision, error) {
	return f.decision, nil
}
func (f *fakeRepo) GetAction(context.Context, string) (authorityledger.Action, error) {
	return f.action, nil
}
func (f *fakeRepo) transition(name, state string) error {
	f.events = append(f.events, name)
	if f.fail == name {
		return errors.New("database unavailable")
	}
	f.sub.State = state
	return nil
}
func (f *fakeRepo) MarkSubmitted(context.Context, authorityledger.OperationID, []byte, time.Time) error {
	return f.transition("submitted", "SUBMITTED")
}
func (f *fakeRepo) RecordBroadcastAmbiguous(context.Context, authorityledger.OperationID, string, time.Time) error {
	return f.transition("ambiguous", "LOCAL_PENDING")
}
func (f *fakeRepo) MarkBroadcastFailed(context.Context, authorityledger.OperationID, string, time.Time) error {
	return f.transition("broadcast_failed", "SUBMISSION_FAILED")
}
func (f *fakeRepo) MarkReceiptPending(context.Context, authorityledger.OperationID, string) error {
	return f.transition("receipt_pending", "SUBMITTED")
}
func (f *fakeRepo) MarkConfirmed(context.Context, authorityledger.OperationID, []byte, int64, []byte) error {
	return f.transition("confirmed", "CONFIRMED")
}
func (f *fakeRepo) MarkReceiptFailed(context.Context, authorityledger.OperationID, []byte, string) error {
	return f.transition("receipt_failed", "SUBMISSION_FAILED")
}
func (f *fakeRepo) MarkRecoveredConfirmed(context.Context, authorityledger.OperationID, []byte, int64, []byte) error {
	return f.transition("recovered", "CONFIRMED")
}
func (f *fakeRepo) RecordReconciliationError(context.Context, authorityledger.OperationID, string) error {
	return f.transition("reconciliation_error", f.sub.State)
}
func (f *fakeRepo) Unresolved(context.Context) ([]authorityledger.Submission, error) {
	if f.sub.State == "CONFIRMED" {
		return nil, nil
	}
	return []authorityledger.Submission{f.sub}, nil
}

type fakeLedger struct {
	repo                  *fakeRepo
	broadcastErr, waitErr error
	receipt               ledger.Receipt
	assertion, response   bool
	assertionValue        ledger.AuthorityAssertion
	responseValue         ledger.ResponseVersion
	lookupStatus          ledger.ChainRecordStatus
	lookupErr             error
	lookupAssertion       ledger.AuthorityAssertion
	lookupResponse        ledger.ResponseVersion
	observeErr            error
}

func (f *fakeLedger) BroadcastAuthorityAssertion(_ context.Context, value ledger.AuthorityAssertion) (ledger.TransactionHash, error) {
	f.repo.events = append(f.repo.events, "broadcast")
	f.assertion = true
	f.assertionValue = value
	return f.receipt.Hash, f.broadcastErr
}
func (f *fakeLedger) BroadcastResponseVersion(_ context.Context, value ledger.ResponseVersion) (ledger.TransactionHash, error) {
	f.repo.events = append(f.repo.events, "broadcast")
	f.response = true
	f.responseValue = value
	return f.receipt.Hash, f.broadcastErr
}
func (f *fakeLedger) ObserveReceipt(context.Context, ledger.TransactionHash) (ledger.Receipt, error) {
	return f.receipt, f.observeErr
}
func (f *fakeLedger) LookupAuthorityAssertion(context.Context, ledger.ID) (ledger.AuthorityAssertionLookup, error) {
	f.repo.events = append(f.repo.events, "lookup")
	return ledger.AuthorityAssertionLookup{Status: f.lookupStatus, Assertion: f.lookupAssertion, Receipt: f.receipt}, f.lookupErr
}
func (f *fakeLedger) LookupResponseVersion(context.Context, ledger.ID) (ledger.ResponseVersionLookup, error) {
	f.repo.events = append(f.repo.events, "lookup")
	return ledger.ResponseVersionLookup{Status: f.lookupStatus, Response: f.lookupResponse, Receipt: f.receipt}, f.lookupErr
}
func (f *fakeLedger) WaitReceipt(context.Context, ledger.TransactionHash) (ledger.Receipt, error) {
	f.repo.events = append(f.repo.events, "wait")
	return f.receipt, f.waitErr
}

func fixture(response bool) (*fakeRepo, *fakeLedger, *Service) {
	var c authorityledger.Commitment
	c[0] = 1
	r := &fakeRepo{sub: authorityledger.Submission{OperationID: "OP-1", OperationType: OperationAuthorityAssertion, DomainVersionID: "EVENT-1", SignerContext: "HHS-OIG-DEMO", IntendedCommitment: c, State: "LOCAL_PENDING"}, assertion: authorityledger.AuthorityAssertion{ID: "EVENT-1", SeriesID: "SERIES-1", EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", Commitment: c}, series: authorityledger.EventSeries{ID: "SERIES-1", AuthorityID: "HHS-OIG-DEMO"}}
	if response {
		r.sub.OperationType = OperationResponseVersion
		r.sub.DomainVersionID = "RESPONSE-V1"
		r.sub.SignerContext = "HOSPITAL-A"
		r.response = authorityledger.ResponseVersion{ID: "RESPONSE-V1", ResponseID: "RESPONSE-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", Commitment: c}
	}
	var tx, b ledger.TransactionHash
	tx[0] = 2
	b[0] = 3
	l := &fakeLedger{repo: r, receipt: ledger.Receipt{Transaction: ledger.Transaction{Hash: tx, BlockHash: b, BlockNumber: 7}, Status: ledger.ReceiptSuccessful}}
	return r, l, New(r, map[string]ledger.SubmissionInterface{r.sub.SignerContext: l})
}

func TestSubmitHappyPathsAndOrdering(t *testing.T) {
	for _, response := range []bool{false, true} {
		r, l, s := fixture(response)
		if err := s.Submit(context.Background(), "OP-1"); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(r.events, []string{"broadcast", "submitted", "wait", "confirmed"}) {
			t.Fatalf("events %v", r.events)
		}
		if r.sub.State != "CONFIRMED" {
			t.Fatal(r.sub.State)
		}
		if response && !l.response {
			t.Fatal("response not broadcast")
		}
		if !response && !l.assertion {
			t.Fatal("assertion not broadcast")
		}
		if response && (l.responseValue.ResponseVersionID.String() != "RESPONSE-V1" || l.responseValue.OrganizationID.String() != "HOSPITAL-A" || l.responseValue.State != ledger.ResponseReceived) {
			t.Fatalf("wrong response payload: %+v", l.responseValue)
		}
		if !response && (l.assertionValue.EventID.String() != "EVENT-1" || l.assertionValue.AuthorityID.String() != "HHS-OIG-DEMO" || l.assertionValue.EventCommitment != ledger.Commitment(r.assertion.Commitment)) {
			t.Fatalf("wrong assertion payload: %+v", l.assertionValue)
		}
	}
}
func TestDefiniteBroadcastFailure(t *testing.T) {
	r, l, s := fixture(false)
	l.broadcastErr = ledger.ErrContractRevert
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ledger.ErrContractRevert) {
		t.Fatal("expected revert")
	}
	if r.sub.State != "SUBMISSION_FAILED" || !reflect.DeepEqual(r.events, []string{"broadcast", "broadcast_failed"}) {
		t.Fatalf("%s %v", r.sub.State, r.events)
	}
}
func TestAmbiguousBroadcastFailureStaysPending(t *testing.T) {
	r, l, s := fixture(false)
	l.broadcastErr = ledger.ErrTransport
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ledger.ErrTransport) {
		t.Fatal("expected transport")
	}
	if r.sub.State != "LOCAL_PENDING" || !reflect.DeepEqual(r.events, []string{"broadcast", "ambiguous"}) {
		t.Fatalf("%s %v", r.sub.State, r.events)
	}
}
func TestReceiptTimeoutStaysSubmitted(t *testing.T) {
	r, l, s := fixture(false)
	l.waitErr = ledger.ErrReceiptTimeout
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ledger.ErrReceiptTimeout) {
		t.Fatal("expected timeout")
	}
	if r.sub.State != "SUBMITTED" || !reflect.DeepEqual(r.events, []string{"broadcast", "submitted", "wait", "receipt_pending"}) {
		t.Fatalf("%s %v", r.sub.State, r.events)
	}
}
func TestReceiptTransportFailureStaysSubmitted(t *testing.T) {
	r, l, s := fixture(false)
	l.waitErr = ledger.ErrTransport
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ledger.ErrTransport) {
		t.Fatal("expected transport error")
	}
	if r.sub.State != "SUBMITTED" || !reflect.DeepEqual(r.events, []string{"broadcast", "submitted", "wait", "receipt_pending"}) {
		t.Fatalf("%s %v", r.sub.State, r.events)
	}
}
func TestRevertedReceiptFails(t *testing.T) {
	r, l, s := fixture(false)
	l.receipt.Status = ledger.ReceiptReverted
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ledger.ErrContractRevert) {
		t.Fatal("expected revert")
	}
	if r.sub.State != "SUBMISSION_FAILED" {
		t.Fatal(r.sub.State)
	}
}
func TestBroadcastPersistenceFailureStopsBeforeWait(t *testing.T) {
	r, _, s := fixture(false)
	r.fail = "submitted"
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ErrPostBroadcastPersistence) {
		t.Fatal("expected persistence error")
	}
	if !reflect.DeepEqual(r.events, []string{"broadcast", "submitted"}) {
		t.Fatal(r.events)
	}
}
func TestConfirmationPersistenceFailureIsTyped(t *testing.T) {
	r, _, s := fixture(false)
	r.fail = "confirmed"
	if !errors.Is(s.Submit(context.Background(), "OP-1"), ErrPostReceiptPersistence) {
		t.Fatal("expected persistence error")
	}
}
