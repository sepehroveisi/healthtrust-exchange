package authorityledger

import (
	"context"
	"errors"
	"time"
)

var ErrSubmissionTransition = errors.New("authority ledger: invalid submission transition")

func (s *Store) MarkSubmitted(ctx context.Context, id OperationID, hash []byte, attempt time.Time) error {
	if len(hash) != 32 {
		return errors.New("transaction hash must be 32 bytes")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET state='SUBMITTED',transaction_hash=$2,attempt_count=attempt_count+1,last_attempt_at=$3,last_error_class=NULL,updated_at=now() WHERE operation_id=$1 AND state='LOCAL_PENDING'`, id, hash, attempt.UTC())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
func (s *Store) RecordBroadcastAmbiguous(ctx context.Context, id OperationID, class string, attempt time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET attempt_count=attempt_count+1,last_attempt_at=$2,last_error_class=$3,updated_at=now() WHERE operation_id=$1 AND state='LOCAL_PENDING'`, id, attempt.UTC(), class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
func (s *Store) MarkBroadcastFailed(ctx context.Context, id OperationID, class string, attempt time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET state='SUBMISSION_FAILED',attempt_count=attempt_count+1,last_attempt_at=$2,last_error_class=$3,updated_at=now() WHERE operation_id=$1 AND state='LOCAL_PENDING'`, id, attempt.UTC(), class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
func (s *Store) MarkReceiptPending(ctx context.Context, id OperationID, class string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET last_error_class=$2,updated_at=now() WHERE operation_id=$1 AND state='SUBMITTED'`, id, class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
func (s *Store) MarkConfirmed(ctx context.Context, id OperationID, hash []byte, block int64, blockHash []byte) error {
	if len(hash) != 32 || len(blockHash) != 32 || block < 0 {
		return errors.New("invalid confirmation metadata")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET state='CONFIRMED',block_number=$3,block_hash=$4,last_error_class=NULL,updated_at=now() WHERE operation_id=$1 AND state='SUBMITTED' AND transaction_hash=$2`, id, hash, block, blockHash)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
func (s *Store) MarkReceiptFailed(ctx context.Context, id OperationID, hash []byte, class string) error {
	if len(hash) != 32 {
		return errors.New("transaction hash must be 32 bytes")
	}
	tag, err := s.pool.Exec(ctx, `UPDATE authority_ledger.ledger_submissions SET state='SUBMISSION_FAILED',last_error_class=$3,updated_at=now() WHERE operation_id=$1 AND state='SUBMITTED' AND transaction_hash=$2`, id, hash, class)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrSubmissionTransition
	}
	return nil
}
