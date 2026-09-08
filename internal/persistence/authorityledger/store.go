package authorityledger

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, url string) (*Store, error) {
	p, e := pgxpool.New(ctx, url)
	if e != nil {
		return nil, e
	}
	if e = p.Ping(ctx); e != nil {
		p.Close()
		return nil, e
	}
	return &Store{p}, nil
}
func (s *Store) Close()              { s.pool.Close() }
func (s *Store) Pool() *pgxpool.Pool { return s.pool }
func (s *Store) CreateOrganization(c context.Context, v Organization) error {
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.organizations(id,type,display_name,active,signer_address) VALUES($1,$2,$3,$4,$5)`, v.ID, v.Type, v.DisplayName, v.Active, v.Signer[:])
	return e
}
func (s *Store) GetOrganization(c context.Context, id OrganizationID) (v Organization, e error) {
	var b []byte
	e = s.pool.QueryRow(c, `SELECT id,type,display_name,active,signer_address FROM authority_ledger.organizations WHERE id=$1`, id).Scan(&v.ID, &v.Type, &v.DisplayName, &v.Active, &b)
	copy(v.Signer[:], b)
	return
}
func (s *Store) CreateAuthority(c context.Context, v Authority) error {
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.authorities(id,type,display_name,active,signer_address) VALUES($1,$2,$3,$4,$5)`, v.ID, v.Type, v.DisplayName, v.Active, v.Signer[:])
	return e
}
func (s *Store) GetAuthority(c context.Context, id AuthorityID) (v Authority, e error) {
	var b []byte
	e = s.pool.QueryRow(c, `SELECT id,type,display_name,active,signer_address FROM authority_ledger.authorities WHERE id=$1`, id).Scan(&v.ID, &v.Type, &v.DisplayName, &v.Active, &b)
	copy(v.Signer[:], b)
	return
}
func (s *Store) CreateSubject(c context.Context, v ProfessionalSubject) error {
	if !json.Valid(v.Details) {
		return errors.New("invalid subject details JSON")
	}
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.professional_subjects(id,display_name,details) VALUES($1,$2,$3)`, v.ID, v.DisplayName, v.Details)
	return e
}
func (s *Store) GetSubject(c context.Context, id SubjectID) (v ProfessionalSubject, e error) {
	e = s.pool.QueryRow(c, `SELECT id,display_name,details FROM authority_ledger.professional_subjects WHERE id=$1`, id).Scan(&v.ID, &v.DisplayName, &v.Details)
	return
}
func (s *Store) CreateEventSeries(c context.Context, v EventSeries) error {
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.event_series(id,subject_id,authority_id) VALUES($1,$2,$3)`, v.ID, v.SubjectID, v.AuthorityID)
	return e
}
func (s *Store) GetEventSeries(c context.Context, id SeriesID) (v EventSeries, e error) {
	e = s.pool.QueryRow(c, `SELECT id,subject_id,authority_id FROM authority_ledger.event_series WHERE id=$1`, id).Scan(&v.ID, &v.SubjectID, &v.AuthorityID)
	return
}
func assertionArgs(v AuthorityAssertion) []any {
	return []any{v.ID, v.SeriesID, v.PreviousID, v.TargetID, v.EventType, v.Kind, v.Effect, v.EffectiveUnixNano, v.SourceDocument, v.Commitment[:], v.CanonicalVersion}
}
func insertAssertionTx(c context.Context, q pgx.Tx, v AuthorityAssertion) error {
	_, e := q.Exec(c, `INSERT INTO authority_ledger.authority_assertions(id,series_id,previous_id,target_id,event_type,assertion_kind,effect,effective_unix_nano,source_document,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, assertionArgs(v)...)
	return e
}
func (s *Store) AppendAssertion(c context.Context, v AuthorityAssertion) error {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = insertAssertionTx(c, tx, v); e != nil {
		return e
	}
	return tx.Commit(c)
}
func (s *Store) GetAssertion(c context.Context, id AssertionID) (v AuthorityAssertion, e error) {
	var p, t *string
	var h []byte
	e = s.pool.QueryRow(c, `SELECT id,series_id,previous_id,target_id,event_type,assertion_kind,effect,effective_unix_nano,source_document,commitment,canonical_version,created_at FROM authority_ledger.authority_assertions WHERE id=$1`, id).Scan(&v.ID, &v.SeriesID, &p, &t, &v.EventType, &v.Kind, &v.Effect, &v.EffectiveUnixNano, &v.SourceDocument, &h, &v.CanonicalVersion, &v.CreatedAt)
	if p != nil {
		x := AssertionID(*p)
		v.PreviousID = &x
	}
	if t != nil {
		x := AssertionID(*t)
		v.TargetID = &x
	}
	if len(h) != 32 && e == nil {
		return v, errors.New("invalid stored commitment")
	}
	copy(v.Commitment[:], h)
	return
}
func (s *Store) AssertionHistory(c context.Context, id SeriesID) (out []AuthorityAssertion, e error) {
	rows, e := s.pool.Query(c, `SELECT id FROM authority_ledger.authority_assertions WHERE series_id=$1 ORDER BY created_at,id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var x AssertionID
		if e = rows.Scan(&x); e != nil {
			return
		}
		v, z := s.GetAssertion(c, x)
		if z != nil {
			return nil, z
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) CreateResponseStream(c context.Context, v ResponseStream) error {
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.response_streams(response_id,event_id,organization_id) VALUES($1,$2,$3)`, v.ID, v.EventID, v.OrganizationID)
	return e
}
func insertResponseTx(c context.Context, q pgx.Tx, v ResponseVersion) error {
	_, e := q.Exec(c, `INSERT INTO authority_ledger.response_versions(version_id,response_id,event_id,organization_id,previous_version_id,state,receipt_unix_nano,policy_version_id,decision_id,action_id,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, v.ID, v.ResponseID, v.EventID, v.OrganizationID, v.PreviousID, v.State, v.ReceiptUnixNano, v.PolicyVersionID, v.DecisionID, v.ActionID, v.Commitment[:], v.CanonicalVersion)
	return e
}
func (s *Store) AppendResponse(c context.Context, v ResponseVersion) error {
	if e := s.validateResponseReferences(c, v); e != nil {
		return e
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = insertResponseTx(c, tx, v); e != nil {
		return e
	}
	return tx.Commit(c)
}
func (s *Store) GetResponse(c context.Context, id ResponseVersionID) (v ResponseVersion, e error) {
	var p *string
	var h []byte
	e = s.pool.QueryRow(c, `SELECT version_id,response_id,event_id,organization_id,previous_version_id,state,receipt_unix_nano,policy_version_id,decision_id,action_id,commitment,canonical_version,created_at FROM authority_ledger.response_versions WHERE version_id=$1`, id).Scan(&v.ID, &v.ResponseID, &v.EventID, &v.OrganizationID, &p, &v.State, &v.ReceiptUnixNano, &v.PolicyVersionID, &v.DecisionID, &v.ActionID, &h, &v.CanonicalVersion, &v.CreatedAt)
	if p != nil {
		x := ResponseVersionID(*p)
		v.PreviousID = &x
	}
	copy(v.Commitment[:], h)
	return
}
func (s *Store) ResponseHistory(c context.Context, id ResponseID) (out []ResponseVersion, e error) {
	rows, e := s.pool.Query(c, `SELECT version_id FROM authority_ledger.response_versions WHERE response_id=$1 ORDER BY created_at,version_id`, id)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var x ResponseVersionID
		rows.Scan(&x)
		v, z := s.GetResponse(c, x)
		if z != nil {
			return nil, z
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) LatestResponse(c context.Context, id ResponseID) (ResponseVersion, error) {
	var x ResponseVersionID
	e := s.pool.QueryRow(c, `SELECT version_id FROM authority_ledger.response_versions WHERE response_id=$1 ORDER BY created_at DESC,version_id DESC LIMIT 1`, id).Scan(&x)
	if e != nil {
		return ResponseVersion{}, e
	}
	return s.GetResponse(c, x)
}

func insertSubmission(c context.Context, q pgx.Tx, v Submission) error {
	if v.State != "LOCAL_PENDING" {
		return errors.New("new submission must be LOCAL_PENDING")
	}
	_, e := q.Exec(c, `INSERT INTO authority_ledger.ledger_submissions(operation_id,operation_type,domain_version_id,contract_action,signer_context,intended_commitment,state) VALUES($1,$2,$3,$4,$5,$6,$7)`, v.OperationID, v.OperationType, v.DomainVersionID, v.ContractAction, v.SignerContext, v.IntendedCommitment[:], v.State)
	return e
}
func (s *Store) CreatePending(c context.Context, v Submission) error {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = insertSubmission(c, tx, v); e != nil {
		return e
	}
	return tx.Commit(c)
}
func (s *Store) CreateAssertionWithPending(c context.Context, a AuthorityAssertion, v Submission) error {
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = insertAssertionTx(c, tx, a); e == nil {
		e = insertSubmission(c, tx, v)
	}
	if e != nil {
		return e
	}
	return tx.Commit(c)
}
func (s *Store) CreateResponseWithPending(c context.Context, a ResponseVersion, v Submission) error {
	if e := s.validateResponseReferences(c, a); e != nil {
		return e
	}
	tx, e := s.pool.Begin(c)
	if e != nil {
		return e
	}
	defer tx.Rollback(c)
	if e = insertResponseTx(c, tx, a); e == nil {
		e = insertSubmission(c, tx, v)
	}
	if e != nil {
		return e
	}
	return tx.Commit(c)
}
func (s *Store) GetSubmission(c context.Context, id OperationID) (v Submission, e error) {
	var h []byte
	e = s.pool.QueryRow(c, `SELECT operation_id,operation_type,domain_version_id,contract_action,signer_context,intended_commitment,state,transaction_hash,block_number,block_hash,attempt_count,last_attempt_at,last_error_class,created_at,updated_at FROM authority_ledger.ledger_submissions WHERE operation_id=$1`, id).Scan(&v.OperationID, &v.OperationType, &v.DomainVersionID, &v.ContractAction, &v.SignerContext, &h, &v.State, &v.TransactionHash, &v.BlockNumber, &v.BlockHash, &v.AttemptCount, &v.LastAttemptAt, &v.LastErrorClass, &v.CreatedAt, &v.UpdatedAt)
	copy(v.IntendedCommitment[:], h)
	return
}
func (s *Store) Unresolved(c context.Context) (out []Submission, e error) {
	rows, e := s.pool.Query(c, `SELECT operation_id FROM authority_ledger.ledger_submissions WHERE state<>'CONFIRMED' ORDER BY created_at`)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	for rows.Next() {
		var id OperationID
		rows.Scan(&id)
		v, z := s.GetSubmission(c, id)
		if z != nil {
			return nil, z
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) Count(c context.Context, table string) (int, error) {
	allowed := map[string]bool{"authority_assertions": true, "response_versions": true, "ledger_submissions": true}
	if !allowed[table] {
		return 0, fmt.Errorf("invalid table")
	}
	var n int
	e := s.pool.QueryRow(c, "SELECT count(*) FROM authority_ledger."+table).Scan(&n)
	return n, e
}
