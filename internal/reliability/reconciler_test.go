package reliability

import (
	"context"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"testing"
	"time"
)

type fakePending struct {
	records []clinical.ClinicalRecord
	commits int
}

func (f *fakePending) ListPendingRecords(context.Context) ([]clinical.ClinicalRecord, error) {
	return f.records, nil
}
func (f *fakePending) SetRecordCommitState(context.Context, string, clinical.CommitState, []byte) error {
	f.commits++
	return nil
}

type fakeChain bool

func (f fakeChain) HasTransaction(string) bool { return bool(f) }

type fakeRetry struct{ count int }

func (f *fakeRetry) RetryPendingRecord(context.Context, clinical.ClinicalRecord) error {
	f.count++
	return nil
}
func TestReconcilerExistingEventFinalizesWithoutDuplicate(t *testing.T) {
	store := &fakePending{records: []clinical.ClinicalRecord{{ID: "r", CommitState: clinical.CommitPending, CreatedAt: time.Now()}}}
	retry := &fakeRetry{}
	r := Reconciler{Store: store, Chain: fakeChain(true), Retrier: retry}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.commits != 1 || retry.count != 0 {
		t.Fatal("existing event was duplicated")
	}
	if err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if retry.count != 0 {
		t.Fatal("reconciliation not idempotent")
	}
}
func TestReconcilerMissingEventRetries(t *testing.T) {
	store := &fakePending{records: []clinical.ClinicalRecord{{ID: "r", CommitState: clinical.CommitPending}}}
	retry := &fakeRetry{}
	if err := (Reconciler{Store: store, Chain: fakeChain(false), Retrier: retry}).Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if retry.count != 1 {
		t.Fatal("pending record not retried")
	}
}
