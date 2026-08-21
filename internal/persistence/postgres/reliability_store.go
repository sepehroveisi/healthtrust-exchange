package postgres

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/reliability"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"time"
)

func (s *Store) BeginIdempotent(ctx context.Context, operation, key, actor string, hash [sha256.Size]byte) (reliability.IdempotencyResult, error) {
	tag, err := s.pool.Exec(ctx, `INSERT INTO idempotency_keys(operation,key,actor_id,request_hash,status) VALUES($1,$2,$3,$4,'Pending') ON CONFLICT DO NOTHING`, operation, key, actor, hash[:])
	if err != nil {
		return reliability.IdempotencyResult{}, err
	}
	if tag.RowsAffected() == 1 {
		return reliability.IdempotencyResult{Owner: true}, nil
	}
	var stored []byte
	var status string
	var response *string
	if err := s.pool.QueryRow(ctx, `SELECT request_hash,status,response_reference FROM idempotency_keys WHERE operation=$1 AND key=$2`, operation, key).Scan(&stored, &status, &response); err != nil {
		return reliability.IdempotencyResult{}, err
	}
	if !bytes.Equal(stored, hash[:]) {
		return reliability.IdempotencyResult{}, reliability.ErrIdempotencyConflict
	}
	if status != "Completed" {
		return reliability.IdempotencyResult{}, reliability.ErrOperationPending
	}
	result := reliability.IdempotencyResult{}
	if response != nil {
		result.ResponseReference = *response
	}
	return result, nil
}
func (s *Store) CompleteIdempotent(ctx context.Context, operation, key, response string) error {
	tag, err := s.pool.Exec(ctx, `UPDATE idempotency_keys SET status='Completed',response_reference=$3,completed_at=now() WHERE operation=$1 AND key=$2 AND status='Pending'`, operation, key, response)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("idempotency completion rejected")
	}
	return nil
}
func (s *Store) ConsumeRequesterProof(ctx context.Context, id, actor, organization, record string, consumed, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO consumed_requester_proofs VALUES($1,$2,$3,$4,$5,$6)`, id, actor, organization, record, consumed, expires)
	if err != nil {
		return security.ErrReplayedRequest
	}
	return nil
}
func (s *Store) ConsumePeerRequest(ctx context.Context, id, node, organization string, consumed, expires time.Time) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO consumed_peer_requests VALUES($1,$2,$3,$4,$5)`, id, node, organization, consumed, expires)
	if err != nil {
		return security.ErrReplayedRequest
	}
	return nil
}
func (s *Store) ListPendingRecords(ctx context.Context) ([]clinical.ClinicalRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,patient_id,author_doctor_id,organization_id,encounter_summary,diagnosis,prescription,created_at_unix_nano,updated_at_unix_nano,commit_state,record_hash FROM clinical_records WHERE commit_state='Pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clinical.ClinicalRecord
	for rows.Next() {
		var v clinical.ClinicalRecord
		var c, u int64
		if err := rows.Scan(&v.ID, &v.PatientID, &v.AuthorDoctorID, &v.OrganizationID, &v.EncounterSummary, &v.Diagnosis, &v.Prescription, &c, &u, &v.CommitState, &v.RecordHash); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(0, c).UTC()
		v.UpdatedAt = time.Unix(0, u).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}
