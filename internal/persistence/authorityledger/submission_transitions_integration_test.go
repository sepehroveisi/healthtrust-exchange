//go:build integration

package authorityledger

import (
	"errors"
	"testing"
	"time"
)

func TestSubmissionTransitionsAndAttemptMetadata(t *testing.T) {
	ctx, store := testStore(t)
	seed(t, ctx, store)
	value := original()
	sub := Submission{OperationID: "OP-TRANSITION", OperationType: "AUTHORITY_ASSERTION", DomainVersionID: string(value.ID), ContractAction: "recordAssertion", SignerContext: "HHS-OIG-DEMO", IntendedCommitment: value.Commitment, State: "LOCAL_PENDING"}
	if err := store.CreateAssertionWithPending(ctx, value, sub); err != nil {
		t.Fatal(err)
	}
	attempt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	tx := make([]byte, 32)
	tx[0] = 1
	blockHash := make([]byte, 32)
	blockHash[0] = 2
	if err := store.MarkSubmitted(ctx, sub.OperationID, tx, attempt); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSubmission(ctx, sub.OperationID)
	if err != nil || got.State != "SUBMITTED" || got.AttemptCount != 1 || got.LastAttemptAt == nil || !got.LastAttemptAt.Equal(attempt) || got.LastErrorClass != nil {
		t.Fatalf("submitted: %+v %v", got, err)
	}
	if err := store.MarkSubmitted(ctx, sub.OperationID, tx, attempt); !errors.Is(err, ErrSubmissionTransition) {
		t.Fatalf("repeated MarkSubmitted: %v", err)
	}
	if err := store.MarkConfirmed(ctx, sub.OperationID, tx, 9, blockHash); err != nil {
		t.Fatal(err)
	}
	got, err = store.GetSubmission(ctx, sub.OperationID)
	if err != nil || got.State != "CONFIRMED" || got.BlockNumber == nil || *got.BlockNumber != 9 {
		t.Fatalf("confirmed: %+v %v", got, err)
	}
	if err := store.MarkReceiptFailed(ctx, sub.OperationID, tx, "RECEIPT_REVERTED"); !errors.Is(err, ErrSubmissionTransition) {
		t.Fatalf("invalid confirmed transition: %v", err)
	}
}

func TestSubmissionFailureTransitions(t *testing.T) {
	ctx, store := testStore(t)
	seed(t, ctx, store)
	first := original()
	sub := Submission{OperationID: "OP-FAIL", OperationType: "AUTHORITY_ASSERTION", DomainVersionID: string(first.ID), ContractAction: "recordAssertion", SignerContext: "HHS-OIG-DEMO", IntendedCommitment: first.Commitment, State: "LOCAL_PENDING"}
	if err := store.CreateAssertionWithPending(ctx, first, sub); err != nil {
		t.Fatal(err)
	}
	attempt := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	if err := store.RecordBroadcastAmbiguous(ctx, sub.OperationID, "BROADCAST_ERROR", attempt); err != nil {
		t.Fatal(err)
	}
	got, _ := store.GetSubmission(ctx, sub.OperationID)
	if got.State != "LOCAL_PENDING" || got.AttemptCount != 1 || got.LastErrorClass == nil || *got.LastErrorClass != "BROADCAST_ERROR" {
		t.Fatalf("ambiguous: %+v", got)
	}
	if err := store.MarkBroadcastFailed(ctx, sub.OperationID, "SIGNING", attempt.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	got, _ = store.GetSubmission(ctx, sub.OperationID)
	if got.State != "SUBMISSION_FAILED" || got.AttemptCount != 2 || got.LastErrorClass == nil || *got.LastErrorClass != "SIGNING" {
		t.Fatalf("failed: %+v", got)
	}
}
