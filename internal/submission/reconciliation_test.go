package submission

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

func reconcilerFixture(response bool) (*fakeRepo, *fakeLedger, *Reconciler) {
	r, l, _ := fixture(response)
	l.lookupAssertion = ledger.AuthorityAssertion{EventID: ledger.MustID("EVENT-1"), EventSeriesID: ledger.MustID("SERIES-1"), AuthorityID: ledger.MustID("HHS-OIG-DEMO"), EventCommitment: ledger.Commitment(r.assertion.Commitment), EventType: ledger.EventTypeExclusion, AssertionKind: ledger.AssertionOriginal, AuthorityEffect: ledger.EffectExclusionActive}
	l.lookupResponse = ledger.ResponseVersion{ResponseID: ledger.MustID("RESPONSE-1"), ResponseVersionID: ledger.MustID("RESPONSE-V1"), EventID: ledger.MustID("EVENT-1"), OrganizationID: ledger.MustID("HOSPITAL-A"), State: ledger.ResponseReceived}
	return r, l, NewReconciler(r, map[string]ledger.ReconciliationInterface{r.sub.SignerContext: l})
}

func TestReconcileLocalPendingAbsentSubmitsSameOperation(t *testing.T) {
	r, _, service := reconcilerFixture(false)
	out, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if err != nil || out != OutcomeSubmitted || r.sub.State != "CONFIRMED" {
		t.Fatalf("%s %+v %v", out, r.sub, err)
	}
	if !reflect.DeepEqual(r.events, []string{"lookup", "broadcast", "submitted", "wait", "confirmed"}) {
		t.Fatal(r.events)
	}
}
func TestReconcileBroadcastBeforeHashPersistenceRecoversWithoutBroadcast(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	l.lookupStatus = ledger.ChainRecordPresent
	out, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if err != nil || out != OutcomeRecovered || r.sub.State != "CONFIRMED" {
		t.Fatalf("%s %s %v", out, r.sub.State, err)
	}
	if !reflect.DeepEqual(r.events, []string{"lookup", "recovered"}) {
		t.Fatal(r.events)
	}
}
func TestReconcileSubmittedPendingThenConfirmed(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	r.sub.State = "SUBMITTED"
	r.sub.TransactionHash = append([]byte(nil), l.receipt.Hash[:]...)
	l.receipt.Status = ledger.ReceiptUnknown
	out, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if !errors.Is(err, ErrReceiptPending) || out != OutcomeStillPending || r.sub.State != "SUBMITTED" {
		t.Fatalf("%s %s %v", out, r.sub.State, err)
	}
	l.receipt.Status = ledger.ReceiptSuccessful
	l.lookupStatus = ledger.ChainRecordPresent
	r.events = nil
	out, err = service.ReconcileOne(context.Background(), r.sub.OperationID)
	if err != nil || out != OutcomeRecovered || r.sub.State != "CONFIRMED" {
		t.Fatalf("%s %s %v", out, r.sub.State, err)
	}
	if !reflect.DeepEqual(r.events, []string{"lookup", "confirmed"}) {
		t.Fatal(r.events)
	}
}
func TestReconcileConfirmationPersistenceFailureThenRepairs(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	r.sub.State = "SUBMITTED"
	r.sub.TransactionHash = append([]byte(nil), l.receipt.Hash[:]...)
	l.lookupStatus = ledger.ChainRecordPresent
	r.fail = "confirmed"
	if _, err := service.ReconcileOne(context.Background(), r.sub.OperationID); !errors.Is(err, ErrLocalPersistence) {
		t.Fatal(err)
	}
	if r.sub.State != "SUBMITTED" {
		t.Fatal(r.sub.State)
	}
	r.fail = ""
	r.events = nil
	out, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if err != nil || out != OutcomeRecovered || r.sub.State != "CONFIRMED" {
		t.Fatalf("%s %s %v", out, r.sub.State, err)
	}
}
func TestReconcileConflictFailsClosed(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	l.lookupStatus = ledger.ChainRecordPresent
	l.lookupAssertion.EffectiveTime = 99
	_, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if !errors.Is(err, ErrOnChainConflict) || r.sub.State != "LOCAL_PENDING" {
		t.Fatalf("%s %v", r.sub.State, err)
	}
	if reflect.DeepEqual(r.events, []string{"lookup", "broadcast"}) {
		t.Fatal("broadcast occurred")
	}
}
func TestReconcileChainUnavailableFailsClosed(t *testing.T) {
	for _, state := range []string{"LOCAL_PENDING", "SUBMITTED"} {
		r, l, service := reconcilerFixture(false)
		r.sub.State = state
		if state == "SUBMITTED" {
			r.sub.TransactionHash = append([]byte(nil), l.receipt.Hash[:]...)
			l.observeErr = ledger.ErrTransport
		} else {
			l.lookupErr = ledger.ErrTransport
		}
		_, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
		if !errors.Is(err, ErrChainUnavailable) || r.sub.State != state {
			t.Fatalf("%s %v", r.sub.State, err)
		}
		for _, event := range r.events {
			if event == "broadcast" {
				t.Fatal("broadcast occurred")
			}
		}
	}
}
func TestReconcileFailedReceipt(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	r.sub.State = "SUBMITTED"
	r.sub.TransactionHash = append([]byte(nil), l.receipt.Hash[:]...)
	l.receipt.Status = ledger.ReceiptReverted
	out, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if !errors.Is(err, ErrExplicitFailedReceipt) || out != OutcomeFailed || r.sub.State != "SUBMISSION_FAILED" {
		t.Fatalf("%s %s %v", out, r.sub.State, err)
	}
}
func TestReconcileFailedAbsentDoesNotRetry(t *testing.T) {
	r, _, service := reconcilerFixture(false)
	r.sub.State = "SUBMISSION_FAILED"
	_, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if !errors.Is(err, ErrRetryNotPermitted) || r.sub.State != "SUBMISSION_FAILED" {
		t.Fatalf("%s %v", r.sub.State, err)
	}
}
func TestReconcileRejectsUnsupportedOnChainIdentifier(t *testing.T) {
	r, _, service := reconcilerFixture(false)
	r.assertion.ID = "EVENT-IDENTIFIER-LONGER-THAN-THIRTY-TWO-BYTES"
	r.sub.DomainVersionID = string(r.assertion.ID)
	_, err := service.ReconcileOne(context.Background(), r.sub.OperationID)
	if !errors.Is(err, ErrUnsupportedIdentifier) {
		t.Fatalf("expected identifier error: %v", err)
	}
	if len(r.events) != 0 {
		t.Fatalf("chain was touched: %v", r.events)
	}
}
func TestReconciliationIsIdempotentAndBatchConverges(t *testing.T) {
	r, l, service := reconcilerFixture(false)
	l.lookupStatus = ledger.ChainRecordPresent
	if results := service.ReconcileUnresolved(context.Background(), 1); len(results) != 1 || results[0].Err != nil {
		t.Fatalf("%+v", results)
	}
	events := len(r.events)
	if out, err := service.ReconcileOne(context.Background(), r.sub.OperationID); err != nil || out != OutcomeAlreadyConfirmed {
		t.Fatalf("%s %v", out, err)
	}
	if len(r.events) != events {
		t.Fatal("idempotent reconciliation touched chain or state")
	}
	if results := service.ReconcileUnresolved(context.Background(), 1); len(results) != 0 {
		t.Fatalf("batch did not converge: %+v", results)
	}
}
