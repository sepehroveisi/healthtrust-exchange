package reliability

import (
	"context"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"log/slog"
)

type PendingStore interface {
	ListPendingRecords(context.Context) ([]clinical.ClinicalRecord, error)
	SetRecordCommitState(context.Context, string, clinical.CommitState, []byte) error
}
type EventChain interface{ HasTransaction(string) bool }
type RecordRetrier interface {
	RetryPendingRecord(context.Context, clinical.ClinicalRecord) error
}
type Reconciler struct {
	Store   PendingStore
	Chain   EventChain
	Retrier RecordRetrier
	Logger  *slog.Logger
}

func (r Reconciler) Run(ctx context.Context) error {
	records, err := r.Store.ListPendingRecords(ctx)
	if err != nil {
		return err
	}
	for _, record := range records {
		eventID := "record-commit-" + record.ID
		if r.Chain.HasTransaction(eventID) {
			if err := r.Store.SetRecordCommitState(ctx, record.ID, clinical.CommitCommitted, record.RecordHash); err != nil {
				return err
			}
			continue
		}
		if err := r.Retrier.RetryPendingRecord(ctx, record); err != nil {
			return err
		}
	}
	return nil
}
