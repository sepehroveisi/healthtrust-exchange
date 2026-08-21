package workflow

import (
	"context"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"time"
)

func (s *Service) operational() (OperationalStore, error) {
	v, ok := s.Store.(OperationalStore)
	if !ok {
		return nil, errors.New("operational persistence unavailable")
	}
	return v, nil
}
func (s *Service) RegisterPatient(ctx context.Context, v clinical.Patient) (clinical.Patient, error) {
	store, err := s.operational()
	if err != nil {
		return v, err
	}
	if v.ID == "" || v.FullName == "" || v.DateOfBirth == "" || v.Gender == "" || v.Phone == "" {
		return v, errors.New("patient profile is incomplete")
	}
	v.OrganizationID = s.OrganizationID
	if v.NetworkPatientID == "" {
		v.NetworkPatientID = v.ID
	}
	v.CreatedAt = time.Now().UTC()
	if err := store.SavePatient(ctx, v); err != nil {
		return v, err
	}
	if err := s.ensureDemoPatientActor(ctx, v); err != nil {
		return v, err
	}
	return v, nil
}
func (s *Service) CheckIn(ctx context.Context, v clinical.Visit) (clinical.Visit, error) {
	store, err := s.operational()
	if err != nil {
		return v, err
	}
	if v.ID == "" || v.PatientID == "" || v.DoctorID == "" || v.Room == "" || v.Reason == "" {
		return v, errors.New("check-in is incomplete")
	}
	patient, err := store.GetPatient(ctx, s.OrganizationID, v.PatientID)
	if err != nil || patient.ID == "" {
		return v, errors.New("patient is not registered at this hospital")
	}
	doctor, err := s.Registry.GetActor(v.DoctorID)
	if err != nil || doctor.Role != identity.RoleDoctor || doctor.OrganizationID != s.OrganizationID {
		return v, errors.New("assigned doctor is invalid")
	}
	v.OrganizationID = s.OrganizationID
	if v.Source == "" {
		v.Source = clinical.VisitNormal
	}
	if v.Source == clinical.VisitReferral {
		cross, ok := s.Store.(CrossHospitalStore)
		if !ok {
			return v, errors.New("referral persistence unavailable")
		}
		ref, e := cross.GetIncomingReferral(ctx, v.ReferralID)
		if e != nil || ref.Referral.NetworkPatientID == "" || ref.Referral.NetworkPatientID != patient.NetworkPatientID || ref.Referral.DestinationOrganizationID != s.OrganizationID {
			return v, errors.New("incoming referral linkage invalid")
		}
	}
	v.Status = clinical.VisitWaiting
	v.CheckedInAt = time.Now().UTC()
	return v, store.SaveVisit(ctx, v)
}
func (s *Service) StartVisit(ctx context.Context, id, doctor string, at time.Time) error {
	store, err := s.operational()
	if err != nil {
		return err
	}
	v, err := store.GetVisit(ctx, id)
	if err != nil || v.DoctorID != doctor || v.OrganizationID != s.OrganizationID {
		return ErrAccessDenied
	}
	visits, err := store.ListVisits(ctx, s.OrganizationID, doctor)
	if err != nil {
		return err
	}
	for _, other := range visits {
		if other.ID != id && other.Status == clinical.VisitInVisit && sameUTCDate(other.CheckedInAt, at) {
			return errors.New("doctor already has an active visit today")
		}
	}
	return store.StartVisit(ctx, id, at.UTC())
}

func sameUTCDate(a, b time.Time) bool {
	a, b = a.UTC(), b.UTC()
	return a.Year() == b.Year() && a.YearDay() == b.YearDay()
}

func (s *Service) SaveVisitDraft(ctx context.Context, id, doctor string, draft clinical.Visit) error {
	store, err := s.operational()
	if err != nil {
		return err
	}
	v, err := store.GetVisit(ctx, id)
	if err != nil || v.DoctorID != doctor || v.OrganizationID != s.OrganizationID || v.Status == clinical.VisitCompleted {
		return ErrAccessDenied
	}
	draft.ID = id
	return store.SaveVisitDraft(ctx, draft)
}
func (s *Service) CompleteVisit(ctx context.Context, id, doctor, recordID string, at time.Time) (clinical.ClinicalRecord, error) {
	store, err := s.operational()
	if err != nil {
		return clinical.ClinicalRecord{}, err
	}
	v, err := store.GetVisit(ctx, id)
	if err != nil || v.DoctorID != doctor || v.OrganizationID != s.OrganizationID || v.Status != clinical.VisitInVisit {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	if v.Diagnosis == "" || v.ClinicalNotes == "" {
		return clinical.ClinicalRecord{}, errors.New("clinical notes and diagnosis are required")
	}
	record := clinical.ClinicalRecord{ID: recordID, PatientID: v.PatientID, AuthorDoctorID: doctor, OrganizationID: s.OrganizationID, EncounterSummary: v.ChiefComplaint + " — " + v.ClinicalNotes, Diagnosis: v.Diagnosis, Prescription: v.Prescription, CreatedAt: at.UTC()}
	record, err = s.CreateClinicalRecord(ctx, record)
	if err != nil {
		return record, err
	}
	if err = store.CompleteVisit(ctx, id, record.ID, at.UTC()); err != nil {
		return record, err
	}
	return record, nil
}
