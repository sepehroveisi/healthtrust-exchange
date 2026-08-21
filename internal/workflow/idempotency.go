package workflow

import (
	"context"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/reliability"
	"time"
)

func (s *Service) CreateClinicalRecordIdempotent(ctx context.Context, key string, record clinical.ClinicalRecord) (clinical.ClinicalRecord, error) {
	store, ok := s.Store.(reliability.IdempotencyStore)
	if !ok {
		return record, errors.New("idempotency store unavailable")
	}
	hash := clinical.HashEvent(record.ID, record.PatientID, record.AuthorDoctorID, record.OrganizationID, record.EncounterSummary, record.Diagnosis, record.Prescription, record.CreatedAt.UTC().String())
	begin, err := store.BeginIdempotent(ctx, "create_clinical_record", key, record.AuthorDoctorID, hash)
	if err != nil {
		return record, err
	}
	if !begin.Owner {
		return s.Store.GetClinicalRecord(ctx, begin.ResponseReference)
	}
	created, err := s.CreateClinicalRecord(ctx, record)
	if err != nil {
		return record, err
	}
	if err := store.CompleteIdempotent(ctx, "create_clinical_record", key, created.ID); err != nil {
		return record, err
	}
	return created, nil
}

func (s *Service) CreateReferralIdempotent(ctx context.Context, key string, v clinical.Referral) (clinical.Referral, error) {
	store, ok := s.Store.(reliability.IdempotencyStore)
	if !ok {
		return v, errors.New("idempotency store unavailable")
	}
	hash := clinical.HashEvent(v.ID, v.PatientID, v.RecordID, v.SourceOrganizationID, v.DestinationOrganizationID, v.CreatedByActorID, v.CreatedAt.UTC().Format(time.RFC3339Nano))
	begin, err := store.BeginIdempotent(ctx, "create_referral", key, v.CreatedByActorID, hash)
	if err != nil {
		return v, err
	}
	if !begin.Owner {
		items, err := s.Store.ListReferrals(ctx, v.PatientID)
		for _, item := range items {
			if item.ID == begin.ResponseReference {
				return item, nil
			}
		}
		return v, err
	}
	created, err := s.CreateReferral(ctx, v)
	if err != nil {
		return v, err
	}
	if err := store.CompleteIdempotent(ctx, "create_referral", key, created.ID); err != nil {
		return v, err
	}
	return created, nil
}
func (s *Service) GrantConsentIdempotent(ctx context.Context, key, requestID, consentID, patientID string, at time.Time) error {
	store, ok := s.Store.(reliability.IdempotencyStore)
	if !ok {
		return errors.New("idempotency store unavailable")
	}
	hash := clinical.HashEvent(requestID, consentID, patientID, at.UTC().Format(time.RFC3339Nano))
	begin, err := store.BeginIdempotent(ctx, "grant_consent", key, patientID, hash)
	if err != nil {
		return err
	}
	if !begin.Owner {
		return nil
	}
	if err := s.GrantConsent(ctx, requestID, consentID, patientID, at); err != nil {
		return err
	}
	return store.CompleteIdempotent(ctx, "grant_consent", key, consentID)
}
func (s *Service) RevokeConsentIdempotent(ctx context.Context, key, consentID, patientID string, at time.Time) error {
	store, ok := s.Store.(reliability.IdempotencyStore)
	if !ok {
		return errors.New("idempotency store unavailable")
	}
	// Revocation time is persisted when the fail-closed transition begins. It is
	// deliberately excluded here so a timeout retry with a new client timestamp
	// resumes the same operation instead of conflicting with it.
	hash := clinical.HashEvent(consentID, patientID)
	begin, err := store.BeginIdempotent(ctx, "revoke_consent", key, patientID, hash)
	if err != nil && !errors.Is(err, reliability.ErrOperationPending) {
		return err
	}
	if err == nil && !begin.Owner {
		return nil
	}
	if err := s.RevokeConsent(ctx, consentID, patientID, at); err != nil {
		return err
	}
	if err := store.CompleteIdempotent(ctx, "revoke_consent", key, consentID); err != nil {
		check, checkErr := store.BeginIdempotent(ctx, "revoke_consent", key, patientID, hash)
		if checkErr == nil && !check.Owner {
			return nil
		}
		return err
	}
	return nil
}
func (s *Service) AuditAccessIdempotent(ctx context.Context, key string, record clinical.ClinicalRecord, requesterID, organizationID string, at time.Time) error {
	store, ok := s.Store.(reliability.IdempotencyStore)
	if !ok {
		return errors.New("idempotency store unavailable")
	}
	hash := clinical.HashEvent(record.ID, requesterID, organizationID, at.UTC().Format(time.RFC3339Nano))
	begin, err := store.BeginIdempotent(ctx, "record_access", key, requesterID, hash)
	if err != nil {
		return err
	}
	if !begin.Owner {
		return nil
	}
	if err := s.AuditAccess(ctx, record, requesterID, organizationID, at); err != nil {
		return err
	}
	return store.CompleteIdempotent(ctx, "record_access", key, record.ID)
}
