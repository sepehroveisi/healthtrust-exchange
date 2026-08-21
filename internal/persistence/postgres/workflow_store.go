package postgres

import (
	"context"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) SaveClinicalRecord(ctx context.Context, v clinical.ClinicalRecord) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO clinical_records VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.PatientID, v.AuthorDoctorID, v.OrganizationID, v.EncounterSummary, v.Diagnosis, v.Prescription, v.CreatedAt.UnixNano(), v.UpdatedAt.UnixNano(), v.CommitState, v.RecordHash)
	return err
}
func (s *Store) GetClinicalRecord(ctx context.Context, id string) (clinical.ClinicalRecord, error) {
	var v clinical.ClinicalRecord
	var created, updated int64
	err := s.pool.QueryRow(ctx, `SELECT id,patient_id,author_doctor_id,organization_id,encounter_summary,diagnosis,prescription,created_at_unix_nano,updated_at_unix_nano,commit_state,record_hash FROM clinical_records WHERE id=$1`, id).Scan(&v.ID, &v.PatientID, &v.AuthorDoctorID, &v.OrganizationID, &v.EncounterSummary, &v.Diagnosis, &v.Prescription, &created, &updated, &v.CommitState, &v.RecordHash)
	v.CreatedAt = time.Unix(0, created).UTC()
	v.UpdatedAt = time.Unix(0, updated).UTC()
	return v, err
}
func (s *Store) ListClinicalRecords(ctx context.Context, patient string) ([]clinical.ClinicalRecord, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,patient_id,author_doctor_id,organization_id,encounter_summary,diagnosis,prescription,created_at_unix_nano,updated_at_unix_nano,commit_state,record_hash FROM clinical_records WHERE ($1='' OR patient_id=$1) ORDER BY created_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]clinical.ClinicalRecord, 0)
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
func (s *Store) SetRecordCommitState(ctx context.Context, id string, state clinical.CommitState, hash []byte) error {
	_, err := s.pool.Exec(ctx, `UPDATE clinical_records SET commit_state=$2,record_hash=$3 WHERE id=$1`, id, state, hash)
	return err
}
func (s *Store) SaveReferral(ctx context.Context, v clinical.Referral) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO referrals(id,patient_id,network_patient_id,source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state,record_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID, v.PatientID, v.NetworkPatientID, v.SourceOrganizationID, v.DestinationOrganizationID, v.CreatedByActorID, v.Status, v.CreatedAt.UnixNano(), v.CommitState, v.RecordID)
	return err
}
func (s *Store) GetReferral(ctx context.Context, id string) (clinical.Referral, error) {
	var v clinical.Referral
	var at int64
	err := s.pool.QueryRow(ctx, `SELECT id,patient_id,network_patient_id,COALESCE(record_id,''),source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state FROM referrals WHERE id=$1`, id).Scan(&v.ID, &v.PatientID, &v.NetworkPatientID, &v.RecordID, &v.SourceOrganizationID, &v.DestinationOrganizationID, &v.CreatedByActorID, &v.Status, &at, &v.CommitState)
	v.CreatedAt = time.Unix(0, at).UTC()
	return v, err
}
func (s *Store) SetReferralStatus(ctx context.Context, id string, status clinical.ReferralStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE referrals SET status=$2 WHERE id=$1`, id, status)
	return err
}
func (s *Store) SetReferralCommitState(ctx context.Context, id string, state clinical.CommitState) error {
	_, err := s.pool.Exec(ctx, `UPDATE referrals SET commit_state=$2 WHERE id=$1`, id, state)
	return err
}
func (s *Store) ListReferrals(ctx context.Context, patient string) ([]clinical.Referral, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,patient_id,network_patient_id,COALESCE(record_id,''),source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state FROM referrals WHERE ($1='' OR patient_id=$1) ORDER BY created_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]clinical.Referral, 0)
	for rows.Next() {
		var v clinical.Referral
		var at int64
		if err := rows.Scan(&v.ID, &v.PatientID, &v.NetworkPatientID, &v.RecordID, &v.SourceOrganizationID, &v.DestinationOrganizationID, &v.CreatedByActorID, &v.Status, &at, &v.CommitState); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(0, at).UTC()
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) ListPendingReferrals(ctx context.Context) ([]clinical.Referral, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,patient_id,network_patient_id,COALESCE(record_id,''),source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state FROM referrals WHERE commit_state='Pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []clinical.Referral
	for rows.Next() {
		var v clinical.Referral
		var at int64
		if err := rows.Scan(&v.ID, &v.PatientID, &v.NetworkPatientID, &v.RecordID, &v.SourceOrganizationID, &v.DestinationOrganizationID, &v.CreatedByActorID, &v.Status, &at, &v.CommitState); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(0, at).UTC()
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) SaveAccessRequest(ctx context.Context, v clinical.AccessRequest) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO access_requests(id,patient_id,record_id,requester_actor_id,requester_organization_id,status,created_at_unix_nano,updated_at_unix_nano,referral_id,network_patient_id,discovery_scope_id,source_organization_id,purpose) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, v.ID, v.PatientID, v.RecordID, v.RequesterActorID, v.RequesterOrganizationID, v.Status, v.CreatedAt.UnixNano(), v.UpdatedAt.UnixNano(), nullString(v.ReferralID), v.NetworkPatientID, v.DiscoveryScopeID, v.SourceOrganizationID, v.Purpose)
	return err
}
func (s *Store) GetAccessRequest(ctx context.Context, id string) (clinical.AccessRequest, error) {
	var v clinical.AccessRequest
	var c, u int64
	err := s.pool.QueryRow(ctx, `SELECT id,COALESCE(referral_id,''),patient_id,network_patient_id,record_id,discovery_scope_id,requester_actor_id,requester_organization_id,source_organization_id,purpose,status,created_at_unix_nano,updated_at_unix_nano FROM access_requests WHERE id=$1`, id).Scan(&v.ID, &v.ReferralID, &v.PatientID, &v.NetworkPatientID, &v.RecordID, &v.DiscoveryScopeID, &v.RequesterActorID, &v.RequesterOrganizationID, &v.SourceOrganizationID, &v.Purpose, &v.Status, &c, &u)
	v.CreatedAt = time.Unix(0, c).UTC()
	v.UpdatedAt = time.Unix(0, u).UTC()
	return v, err
}
func (s *Store) SetAccessRequestStatus(ctx context.Context, id string, status clinical.AccessStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE access_requests SET status=$2,updated_at_unix_nano=$3 WHERE id=$1`, id, status, time.Now().UTC().UnixNano())
	return err
}
func (s *Store) ListAccessRequests(ctx context.Context, patient string) ([]clinical.AccessRequest, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,COALESCE(referral_id,''),patient_id,network_patient_id,record_id,discovery_scope_id,requester_actor_id,requester_organization_id,source_organization_id,purpose,status,created_at_unix_nano,updated_at_unix_nano FROM access_requests WHERE ($1='' OR patient_id=$1) ORDER BY created_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]clinical.AccessRequest, 0)
	for rows.Next() {
		var v clinical.AccessRequest
		var c, u int64
		if err := rows.Scan(&v.ID, &v.ReferralID, &v.PatientID, &v.NetworkPatientID, &v.RecordID, &v.DiscoveryScopeID, &v.RequesterActorID, &v.RequesterOrganizationID, &v.SourceOrganizationID, &v.Purpose, &v.Status, &c, &u); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(0, c).UTC()
		v.UpdatedAt = time.Unix(0, u).UTC()
		result = append(result, v)
	}
	return result, rows.Err()
}
func (s *Store) SaveConsent(ctx context.Context, v clinical.Consent) error {
	var revoked, expires any
	if v.RevokedAt != nil {
		revoked = v.RevokedAt.UnixNano()
	}
	if v.ExpiresAt != nil {
		expires = v.ExpiresAt.UnixNano()
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO consents(id,access_request_id,patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano,referral_id) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, v.ID, v.AccessRequestID, v.PatientID, v.RecordID, v.GrantedToActorID, v.GrantedToOrganizationID, v.Status, v.GrantedAt.UnixNano(), revoked, expires, v.ReferralID)
	return err
}
func (s *Store) GetConsent(ctx context.Context, id string) (clinical.Consent, error) {
	return s.scanConsent(s.pool.QueryRow(ctx, `SELECT id,access_request_id,COALESCE(referral_id,''),patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano FROM consents WHERE id=$1`, id))
}

type scanner interface{ Scan(...any) error }

func (s *Store) scanConsent(row scanner) (clinical.Consent, error) {
	var v clinical.Consent
	var granted int64
	var revoked, expires *int64
	err := row.Scan(&v.ID, &v.AccessRequestID, &v.ReferralID, &v.PatientID, &v.RecordID, &v.GrantedToActorID, &v.GrantedToOrganizationID, &v.Status, &granted, &revoked, &expires)
	v.GrantedAt = time.Unix(0, granted).UTC()
	if revoked != nil {
		x := time.Unix(0, *revoked).UTC()
		v.RevokedAt = &x
	}
	if expires != nil {
		x := time.Unix(0, *expires).UTC()
		v.ExpiresAt = &x
	}
	return v, err
}
func (s *Store) ActivateConsent(ctx context.Context, id string) error {
	_, err := s.pool.Exec(ctx, `UPDATE consents SET status='Active' WHERE id=$1`, id)
	return err
}
func (s *Store) BeginConsentRevocation(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE consents SET status='RevocationPending',revoked_at_unix_nano=$2 WHERE id=$1 AND status='Active'`, id, at.UnixNano())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("active consent not found")
	}
	return nil
}
func (s *Store) RevokeConsent(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE consents SET status='Revoked',revoked_at_unix_nano=$2 WHERE id=$1 AND status IN ('RevocationPending','Revoked')`, id, at.UnixNano())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("revocation finalization rejected")
	}
	return nil
}
func (s *Store) FindActiveConsent(ctx context.Context, patient, record, actor, organization string, at time.Time) (clinical.Consent, error) {
	v, err := s.scanConsent(s.pool.QueryRow(ctx, `SELECT id,access_request_id,COALESCE(referral_id,''),patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano FROM consents WHERE patient_id=$1 AND record_id=$2 AND granted_to_actor_id=$3 AND granted_to_organization_id=$4 AND status='Active' AND (expires_at_unix_nano IS NULL OR expires_at_unix_nano>$5)`, patient, record, actor, organization, at.UnixNano()))
	if errors.Is(err, pgx.ErrNoRows) {
		return v, errors.New("active consent not found")
	}
	return v, err
}
func (s *Store) ListPendingConsents(ctx context.Context) ([]clinical.Consent, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,access_request_id,COALESCE(referral_id,''),patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano FROM consents WHERE status='Pending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]clinical.Consent, 0)
	for rows.Next() {
		v, err := s.scanConsent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) ListPendingRevocations(ctx context.Context) ([]clinical.Consent, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,access_request_id,COALESCE(referral_id,''),patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano FROM consents WHERE status='RevocationPending'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]clinical.Consent, 0)
	for rows.Next() {
		v, err := s.scanConsent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) ListConsents(ctx context.Context, patient string) ([]clinical.Consent, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,access_request_id,COALESCE(referral_id,''),patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status,granted_at_unix_nano,revoked_at_unix_nano,expires_at_unix_nano FROM consents WHERE ($1='' OR patient_id=$1) ORDER BY granted_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []clinical.Consent
	for rows.Next() {
		v, err := s.scanConsent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) SaveIncomingReferral(ctx context.Context, v clinical.IncomingReferral) error {
	r := v.Referral
	_, err := s.pool.Exec(ctx, `INSERT INTO incoming_referrals(id,patient_id,network_patient_id,record_id,source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state,transaction_id,received_at_unix_nano) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, r.ID, r.PatientID, r.NetworkPatientID, r.RecordID, r.SourceOrganizationID, r.DestinationOrganizationID, r.CreatedByActorID, r.Status, r.CreatedAt.UnixNano(), r.CommitState, v.TransactionID, v.ReceivedAt.UnixNano())
	return err
}
func scanIncoming(row scanner) (clinical.IncomingReferral, error) {
	var v clinical.IncomingReferral
	var c, r int64
	err := row.Scan(&v.Referral.ID, &v.Referral.PatientID, &v.Referral.NetworkPatientID, &v.Referral.RecordID, &v.Referral.SourceOrganizationID, &v.Referral.DestinationOrganizationID, &v.Referral.CreatedByActorID, &v.Referral.Status, &c, &v.Referral.CommitState, &v.TransactionID, &r)
	v.Referral.CreatedAt = time.Unix(0, c).UTC()
	v.ReceivedAt = time.Unix(0, r).UTC()
	return v, err
}
func (s *Store) GetIncomingReferral(ctx context.Context, id string) (clinical.IncomingReferral, error) {
	return scanIncoming(s.pool.QueryRow(ctx, `SELECT id,patient_id,network_patient_id,record_id,source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state,transaction_id,received_at_unix_nano FROM incoming_referrals WHERE id=$1`, id))
}
func (s *Store) ListIncomingReferrals(ctx context.Context, patient string) ([]clinical.IncomingReferral, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,patient_id,network_patient_id,record_id,source_organization_id,destination_organization_id,created_by_actor_id,status,created_at_unix_nano,commit_state,transaction_id,received_at_unix_nano FROM incoming_referrals WHERE ($1='' OR patient_id=$1 OR network_patient_id=$1) ORDER BY received_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clinical.IncomingReferral{}
	for rows.Next() {
		v, e := scanIncoming(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SetIncomingReferralStatus(ctx context.Context, id string, status clinical.ReferralStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE incoming_referrals SET status=$2 WHERE id=$1`, id, status)
	return err
}
func (s *Store) SaveOutgoingAccessRequest(ctx context.Context, v clinical.OutgoingAccessRequest) error {
	q := v.Request
	_, err := s.pool.Exec(ctx, `INSERT INTO outgoing_access_requests(id,referral_id,patient_id,record_id,requester_actor_id,requester_organization_id,source_organization_id,destination_organization_id,status,created_at_unix_nano,updated_at_unix_nano,network_patient_id,discovery_scope_id,purpose) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`, q.ID, nullString(q.ReferralID), q.PatientID, nullString(q.RecordID), q.RequesterActorID, q.RequesterOrganizationID, v.SourceOrganizationID, v.DestinationOrganizationID, q.Status, q.CreatedAt.UnixNano(), q.UpdatedAt.UnixNano(), q.NetworkPatientID, q.DiscoveryScopeID, q.Purpose)
	return err
}
func scanOutgoing(row scanner) (clinical.OutgoingAccessRequest, error) {
	var v clinical.OutgoingAccessRequest
	var c, u int64
	err := row.Scan(&v.Request.ID, &v.Request.ReferralID, &v.Request.PatientID, &v.Request.RecordID, &v.Request.RequesterActorID, &v.Request.RequesterOrganizationID, &v.SourceOrganizationID, &v.DestinationOrganizationID, &v.Request.Status, &c, &u, &v.Request.NetworkPatientID, &v.Request.DiscoveryScopeID, &v.Request.Purpose)
	v.Request.CreatedAt = time.Unix(0, c).UTC()
	v.Request.UpdatedAt = time.Unix(0, u).UTC()
	v.Request.SourceOrganizationID = v.DestinationOrganizationID
	return v, err
}
func (s *Store) GetOutgoingAccessRequest(ctx context.Context, id string) (clinical.OutgoingAccessRequest, error) {
	return scanOutgoing(s.pool.QueryRow(ctx, `SELECT id,COALESCE(referral_id,''),patient_id,COALESCE(record_id,''),requester_actor_id,requester_organization_id,source_organization_id,destination_organization_id,status,created_at_unix_nano,updated_at_unix_nano,network_patient_id,discovery_scope_id,purpose FROM outgoing_access_requests WHERE id=$1`, id))
}
func (s *Store) ListOutgoingAccessRequests(ctx context.Context, patient string) ([]clinical.OutgoingAccessRequest, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,COALESCE(referral_id,''),patient_id,COALESCE(record_id,''),requester_actor_id,requester_organization_id,source_organization_id,destination_organization_id,status,created_at_unix_nano,updated_at_unix_nano,network_patient_id,discovery_scope_id,purpose FROM outgoing_access_requests WHERE ($1='' OR patient_id=$1) ORDER BY created_at_unix_nano`, patient)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clinical.OutgoingAccessRequest{}
	for rows.Next() {
		v, e := scanOutgoing(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SetOutgoingAccessRequestStatus(ctx context.Context, id string, status clinical.AccessStatus) error {
	_, err := s.pool.Exec(ctx, `UPDATE outgoing_access_requests SET status=$2,updated_at_unix_nano=$3 WHERE id=$1`, id, status, time.Now().UTC().UnixNano())
	return err
}
func (s *Store) SetOutgoingAccessRequestResolution(ctx context.Context, id, recordID string) error {
	_, err := s.pool.Exec(ctx, `UPDATE outgoing_access_requests SET record_id=$2,updated_at_unix_nano=$3 WHERE id=$1`, id, recordID, time.Now().UTC().UnixNano())
	return err
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
