package postgres

import (
	"context"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"time"
)

func (s *Store) SavePatient(ctx context.Context, v clinical.Patient) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO patients(id,organization_id,network_patient_id,full_name,date_of_birth,gender,phone,address,emergency_contact,created_at_unix_nano)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID, v.OrganizationID, v.NetworkPatientID, v.FullName, v.DateOfBirth, v.Gender, v.Phone, v.Address, v.EmergencyContact, v.CreatedAt.UnixNano())
	return err
}
func (s *Store) GetPatient(ctx context.Context, org, id string) (clinical.Patient, error) {
	var v clinical.Patient
	var at int64
	err := s.pool.QueryRow(ctx, `SELECT id,organization_id,network_patient_id,full_name,date_of_birth,gender,phone,address,emergency_contact,created_at_unix_nano FROM patients WHERE organization_id=$1 AND id=$2`, org, id).Scan(&v.ID, &v.OrganizationID, &v.NetworkPatientID, &v.FullName, &v.DateOfBirth, &v.Gender, &v.Phone, &v.Address, &v.EmergencyContact, &at)
	v.CreatedAt = time.Unix(0, at).UTC()
	return v, err
}
func (s *Store) GetPatientByNetworkID(ctx context.Context, org, networkID string) (clinical.Patient, error) {
	var v clinical.Patient
	var at int64
	err := s.pool.QueryRow(ctx, `SELECT id,organization_id,network_patient_id,full_name,date_of_birth,gender,phone,address,emergency_contact,created_at_unix_nano FROM patients WHERE organization_id=$1 AND network_patient_id=$2`, org, networkID).Scan(&v.ID, &v.OrganizationID, &v.NetworkPatientID, &v.FullName, &v.DateOfBirth, &v.Gender, &v.Phone, &v.Address, &v.EmergencyContact, &at)
	v.CreatedAt = time.Unix(0, at).UTC()
	return v, err
}
func (s *Store) ListPatients(ctx context.Context, org, q string) ([]clinical.Patient, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,organization_id,network_patient_id,full_name,date_of_birth,gender,phone,address,emergency_contact,created_at_unix_nano FROM patients WHERE organization_id=$1 AND ($2='' OR id ILIKE '%'||$2||'%' OR full_name ILIKE '%'||$2||'%' OR phone ILIKE '%'||$2||'%') ORDER BY full_name`, org, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clinical.Patient{}
	for rows.Next() {
		var v clinical.Patient
		var at int64
		if err := rows.Scan(&v.ID, &v.OrganizationID, &v.NetworkPatientID, &v.FullName, &v.DateOfBirth, &v.Gender, &v.Phone, &v.Address, &v.EmergencyContact, &at); err != nil {
			return nil, err
		}
		v.CreatedAt = time.Unix(0, at).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) SaveVisit(ctx context.Context, v clinical.Visit) error {
	var ref any
	if v.ReferralID != "" {
		ref = v.ReferralID
	}
	_, err := s.pool.Exec(ctx, `INSERT INTO visits(id,patient_id,organization_id,doctor_id,room,reason,source,referral_id,status,checked_in_at_unix_nano)VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID, v.PatientID, v.OrganizationID, v.DoctorID, v.Room, v.Reason, v.Source, ref, v.Status, v.CheckedInAt.UnixNano())
	return err
}
func scanVisit(row scanner) (clinical.Visit, error) {
	var v clinical.Visit
	var checked int64
	var started, completed *int64
	var referral, record *string
	err := row.Scan(&v.ID, &v.PatientID, &v.OrganizationID, &v.DoctorID, &v.Room, &v.Reason, &v.Source, &referral, &v.Status, &checked, &started, &completed, &v.ChiefComplaint, &v.ClinicalNotes, &v.Diagnosis, &v.Prescription, &v.FollowUpPlan, &record)
	if referral != nil {
		v.ReferralID = *referral
	}
	if record != nil {
		v.RecordID = *record
	}
	v.CheckedInAt = time.Unix(0, checked).UTC()
	if started != nil {
		x := time.Unix(0, *started).UTC()
		v.StartedAt = &x
	}
	if completed != nil {
		x := time.Unix(0, *completed).UTC()
		v.CompletedAt = &x
	}
	return v, err
}

const visitColumns = `id,patient_id,organization_id,doctor_id,room,reason,source,referral_id,status,checked_in_at_unix_nano,started_at_unix_nano,completed_at_unix_nano,chief_complaint,clinical_notes,diagnosis,prescription,follow_up_plan,record_id`

func (s *Store) GetVisit(ctx context.Context, id string) (clinical.Visit, error) {
	return scanVisit(s.pool.QueryRow(ctx, `SELECT `+visitColumns+` FROM visits WHERE id=$1`, id))
}
func (s *Store) ListVisits(ctx context.Context, org, doctor string) ([]clinical.Visit, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+visitColumns+` FROM visits WHERE organization_id=$1 AND ($2='' OR doctor_id=$2) ORDER BY checked_in_at_unix_nano`, org, doctor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []clinical.Visit{}
	for rows.Next() {
		v, e := scanVisit(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) StartVisit(ctx context.Context, id string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE visits SET status='InVisit',started_at_unix_nano=$2 WHERE id=$1 AND status='Waiting'`, id, at.UnixNano())
	if err == nil && tag.RowsAffected() != 1 {
		return context.Canceled
	}
	return err
}
func (s *Store) SaveVisitDraft(ctx context.Context, v clinical.Visit) error {
	_, err := s.pool.Exec(ctx, `UPDATE visits SET chief_complaint=$2,clinical_notes=$3,diagnosis=$4,prescription=$5,follow_up_plan=$6 WHERE id=$1 AND status<>'Completed'`, v.ID, v.ChiefComplaint, v.ClinicalNotes, v.Diagnosis, v.Prescription, v.FollowUpPlan)
	return err
}
func (s *Store) CompleteVisit(ctx context.Context, id, record string, at time.Time) error {
	tag, err := s.pool.Exec(ctx, `UPDATE visits SET status='Completed',completed_at_unix_nano=$2,record_id=$3 WHERE id=$1 AND status='InVisit'`, id, at.UnixNano(), record)
	if err == nil && tag.RowsAffected() != 1 {
		return context.Canceled
	}
	return err
}
