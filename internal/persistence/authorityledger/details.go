package authorityledger

import (
	"context"
	"errors"
)

func validCommitment(c Commitment) error {
	var zero Commitment
	if c == zero {
		return errors.New("commitment must not be zero")
	}
	return nil
}

func (s *Store) CreatePolicy(c context.Context, v Policy) error {
	if e := validCommitment(v.Commitment); e != nil {
		return e
	}
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.policies(id,version_id,organization_id,effective_from_unix_nano,effective_until_unix_nano,policy_text,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, v.ID, v.VersionID, v.OrganizationID, v.EffectiveFromUnixNano, v.EffectiveUntilUnixNano, v.Text, v.Commitment[:], v.CanonicalVersion)
	return e
}
func (s *Store) GetPolicy(c context.Context, id string) (v Policy, e error) {
	var h []byte
	e = s.pool.QueryRow(c, `SELECT id,version_id,organization_id,effective_from_unix_nano,effective_until_unix_nano,policy_text,commitment,canonical_version FROM authority_ledger.policies WHERE version_id=$1`, id).Scan(&v.ID, &v.VersionID, &v.OrganizationID, &v.EffectiveFromUnixNano, &v.EffectiveUntilUnixNano, &v.Text, &h, &v.CanonicalVersion)
	copy(v.Commitment[:], h)
	return
}
func (s *Store) CreateDecision(c context.Context, v Decision) error {
	if e := validCommitment(v.Commitment); e != nil {
		return e
	}
	var policyOrg OrganizationID
	if e := s.pool.QueryRow(c, `SELECT organization_id FROM authority_ledger.policies WHERE version_id=$1`, v.PolicyVersionID).Scan(&policyOrg); e != nil {
		return e
	}
	if policyOrg != v.OrganizationID {
		return errors.New("decision organization does not match policy")
	}
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.decisions(id,event_id,organization_id,policy_version_id,decision_code,explanation,reviewer_reference,decided_unix_nano,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID, v.EventID, v.OrganizationID, v.PolicyVersionID, v.Code, v.Explanation, v.ReviewerReference, v.DecidedUnixNano, v.Commitment[:], v.CanonicalVersion)
	return e
}
func (s *Store) GetDecision(c context.Context, id string) (v Decision, e error) {
	var h []byte
	e = s.pool.QueryRow(c, `SELECT id,event_id,organization_id,policy_version_id,decision_code,explanation,reviewer_reference,decided_unix_nano,commitment,canonical_version FROM authority_ledger.decisions WHERE id=$1`, id).Scan(&v.ID, &v.EventID, &v.OrganizationID, &v.PolicyVersionID, &v.Code, &v.Explanation, &v.ReviewerReference, &v.DecidedUnixNano, &h, &v.CanonicalVersion)
	copy(v.Commitment[:], h)
	return
}
func (s *Store) CreateAction(c context.Context, v Action) error {
	if e := validCommitment(v.Commitment); e != nil {
		return e
	}
	var event AssertionID
	var org OrganizationID
	if e := s.pool.QueryRow(c, `SELECT event_id,organization_id FROM authority_ledger.decisions WHERE id=$1`, v.DecisionID).Scan(&event, &org); e != nil {
		return e
	}
	if event != v.EventID || org != v.OrganizationID {
		return errors.New("action context does not match decision")
	}
	_, e := s.pool.Exec(c, `INSERT INTO authority_ledger.actions(id,event_id,organization_id,decision_id,action_code,explanation,operator_reference,completed_unix_nano,supporting_evidence,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.EventID, v.OrganizationID, v.DecisionID, v.Code, v.Explanation, v.OperatorReference, v.CompletedUnixNano, v.SupportingEvidence, v.Commitment[:], v.CanonicalVersion)
	return e
}
func (s *Store) GetAction(c context.Context, id string) (v Action, e error) {
	var h []byte
	e = s.pool.QueryRow(c, `SELECT id,event_id,organization_id,decision_id,action_code,explanation,operator_reference,completed_unix_nano,supporting_evidence,commitment,canonical_version FROM authority_ledger.actions WHERE id=$1`, id).Scan(&v.ID, &v.EventID, &v.OrganizationID, &v.DecisionID, &v.Code, &v.Explanation, &v.OperatorReference, &v.CompletedUnixNano, &v.SupportingEvidence, &h, &v.CanonicalVersion)
	copy(v.Commitment[:], h)
	return
}
func (s *Store) GetResponseStream(c context.Context, id ResponseID) (v ResponseStream, e error) {
	e = s.pool.QueryRow(c, `SELECT response_id,event_id,organization_id FROM authority_ledger.response_streams WHERE response_id=$1`, id).Scan(&v.ID, &v.EventID, &v.OrganizationID)
	return
}
func (s *Store) GetSubmissionByDomain(c context.Context, kind, id string) (Submission, error) {
	var op OperationID
	if e := s.pool.QueryRow(c, `SELECT operation_id FROM authority_ledger.ledger_submissions WHERE operation_type=$1 AND domain_version_id=$2`, kind, id).Scan(&op); e != nil {
		return Submission{}, e
	}
	return s.GetSubmission(c, op)
}
