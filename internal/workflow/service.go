package workflow

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"log/slog"
	"net/http"
	"time"
)

var (
	ErrAccessDenied      = errors.New("record access denied")
	ErrIntegrityMismatch = errors.New("clinical record integrity mismatch")
)

type Node interface {
	SubmitTransaction(blockchain.Transaction) error
	CreateBlock(context.Context, time.Time) (blockchain.Block, error)
	HasTransaction(string) bool
}

type CrossHospitalStore interface {
	GetReferral(context.Context, string) (clinical.Referral, error)
	SetReferralStatus(context.Context, string, clinical.ReferralStatus) error
	SaveIncomingReferral(context.Context, clinical.IncomingReferral) error
	GetIncomingReferral(context.Context, string) (clinical.IncomingReferral, error)
	ListIncomingReferrals(context.Context, string) ([]clinical.IncomingReferral, error)
	SetIncomingReferralStatus(context.Context, string, clinical.ReferralStatus) error
	SaveOutgoingAccessRequest(context.Context, clinical.OutgoingAccessRequest) error
	GetOutgoingAccessRequest(context.Context, string) (clinical.OutgoingAccessRequest, error)
	ListOutgoingAccessRequests(context.Context, string) ([]clinical.OutgoingAccessRequest, error)
	SetOutgoingAccessRequestStatus(context.Context, string, clinical.AccessStatus) error
	SetOutgoingAccessRequestResolution(context.Context, string, string) error
}
type OperationalStore interface {
	SavePatient(context.Context, clinical.Patient) error
	GetPatient(context.Context, string, string) (clinical.Patient, error)
	ListPatients(context.Context, string, string) ([]clinical.Patient, error)
	GetPatientByNetworkID(context.Context, string, string) (clinical.Patient, error)
	SaveVisit(context.Context, clinical.Visit) error
	GetVisit(context.Context, string) (clinical.Visit, error)
	ListVisits(context.Context, string, string) ([]clinical.Visit, error)
	StartVisit(context.Context, string, time.Time) error
	SaveVisitDraft(context.Context, clinical.Visit) error
	CompleteVisit(context.Context, string, string, time.Time) error
}

func (s *Service) PendingOperations(ctx context.Context) ([]clinical.ClinicalRecord, error) {
	store, ok := s.Store.(interface {
		ListPendingRecords(context.Context) ([]clinical.ClinicalRecord, error)
	})
	if !ok {
		return nil, errors.New("pending operation store unavailable")
	}
	return store.ListPendingRecords(ctx)
}
func (s *Service) Reconcile(ctx context.Context) (int, error) {
	records, err := s.PendingOperations(ctx)
	if err != nil {
		return 0, err
	}
	count := 0
	for _, record := range records {
		if s.Node.HasTransaction("record-commit-" + record.ID) {
			if err := s.Store.SetRecordCommitState(ctx, record.ID, clinical.CommitCommitted, record.RecordHash); err != nil {
				return count, err
			}
		} else if err := s.RetryPendingRecord(ctx, record); err != nil {
			return count, err
		}
		count++
	}
	referrals, err := s.Store.ListPendingReferrals(ctx)
	if err != nil {
		return count, err
	}
	for _, v := range referrals {
		if s.Node.HasTransaction("referral-" + v.ID) {
			if err := s.Store.SetReferralCommitState(ctx, v.ID, clinical.CommitCommitted); err != nil {
				return count, err
			}
		} else if err := s.RetryPendingReferral(ctx, v); err != nil {
			return count, err
		}
		count++
	}
	consents, err := s.Store.ListPendingConsents(ctx)
	if err != nil {
		return count, err
	}
	for _, v := range consents {
		if s.Node.HasTransaction("consent-grant-" + v.ID) {
			if err := s.Store.ActivateConsent(ctx, v.ID); err != nil {
				return count, err
			}
		} else if err := s.RetryPendingConsent(ctx, v); err != nil {
			return count, err
		}
		count++
	}
	revocations, err := s.Store.ListPendingRevocations(ctx)
	if err != nil {
		return count, err
	}
	for _, v := range revocations {
		if err := s.RetryPendingRevocation(ctx, v); err != nil {
			return count, err
		}
		count++
	}
	return count, nil
}

type Service struct {
	Store          Store
	Node           Node
	Registry       identity.Registry
	Keys           map[string]ed25519.PrivateKey
	Logger         *slog.Logger
	NodeID         string
	OrganizationID string
	TrustedPeers   map[string]security.Peer
	PeerURLs       map[string]string
	NodeKey        ed25519.PrivateKey
	HTTPClient     *http.Client
}

func (s *Service) VerifyAndConsumePeer(ctx context.Context, envelope security.PeerEnvelope, method, path string, body []byte, now time.Time) error {
	peer, ok := s.TrustedPeers[envelope.SourceNodeID]
	if !ok {
		return security.ErrUnauthorized
	}
	if err := security.VerifyPeerEnvelope(envelope, peer, s.NodeID, method, path, body, now); err != nil {
		return err
	}
	consumer, ok := s.Store.(interface {
		ConsumePeerRequest(context.Context, string, string, string, time.Time, time.Time) error
	})
	if !ok {
		return errors.New("peer replay store unavailable")
	}
	return consumer.ConsumePeerRequest(ctx, envelope.RequestID, envelope.SourceNodeID, envelope.SourceOrganizationID, now, envelope.ExpiresAt)
}

func (s *Service) commit(ctx context.Context, tx blockchain.Transaction, actorID string) (blockchain.Block, error) {
	key, ok := s.Keys[actorID]
	if !ok {
		return blockchain.Block{}, fmt.Errorf("private key unavailable for %s", actorID)
	}
	if err := tx.Sign(key); err != nil {
		return blockchain.Block{}, err
	}
	if err := s.Node.SubmitTransaction(tx); err != nil {
		return blockchain.Block{}, err
	}
	return s.Node.CreateBlock(ctx, tx.Timestamp)
}
func (s *Service) CreateClinicalRecord(ctx context.Context, record clinical.ClinicalRecord) (clinical.ClinicalRecord, error) {
	actor, err := s.Registry.GetActor(record.AuthorDoctorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != record.OrganizationID {
		return record, errors.New("author is not an authorized doctor")
	}
	record.CreatedAt = record.CreatedAt.UTC()
	record.UpdatedAt = record.CreatedAt
	record.CommitState = clinical.CommitPending
	hash := clinical.HashRecord(record)
	record.RecordHash = hash[:]
	if err := s.Store.SaveClinicalRecord(ctx, record); err != nil {
		return record, err
	}
	tx := blockchain.Transaction{ID: "record-commit-" + record.ID, Type: blockchain.RecordCommitted, ActorID: record.AuthorDoctorID, OrganizationID: record.OrganizationID, ResourceID: record.ID, PayloadHash: hash[:], Timestamp: record.CreatedAt}
	s.Logger.Info("clinical_record_persisted", "node_id", s.NodeID, "organization_id", record.OrganizationID, "record_id", record.ID, "record_hash", fmt.Sprintf("%x", hash), "commit_state", record.CommitState)
	block, err := s.commit(ctx, tx, record.AuthorDoctorID)
	if err != nil {
		_ = s.Store.SetRecordCommitState(ctx, record.ID, clinical.CommitFailed, hash[:])
		return record, err
	}
	if err := s.Store.SetRecordCommitState(ctx, record.ID, clinical.CommitCommitted, hash[:]); err != nil {
		return record, err
	}
	record.CommitState = clinical.CommitCommitted
	s.Logger.Info("clinical_record_created", "node_id", s.NodeID, "organization_id", record.OrganizationID, "record_id", record.ID, "record_hash", fmt.Sprintf("%x", hash), "transaction_id", tx.ID, "event_type", tx.Type, "block_height", block.Height, "block_hash", fmt.Sprintf("%x", block.Hash), "commit_state", record.CommitState)
	return record, nil
}

// RetryPendingRecord commits a previously persisted Pending record without inserting it again.
func (s *Service) RetryPendingRecord(ctx context.Context, record clinical.ClinicalRecord) error {
	if record.CommitState != clinical.CommitPending {
		return nil
	}
	hash := clinical.HashRecord(record)
	tx := blockchain.Transaction{ID: "record-commit-" + record.ID, Type: blockchain.RecordCommitted, ActorID: record.AuthorDoctorID, OrganizationID: record.OrganizationID, ResourceID: record.ID, PayloadHash: hash[:], Timestamp: record.CreatedAt}
	if _, err := s.commit(ctx, tx, record.AuthorDoctorID); err != nil {
		return err
	}
	return s.Store.SetRecordCommitState(ctx, record.ID, clinical.CommitCommitted, hash[:])
}
func (s *Service) RetryPendingReferral(ctx context.Context, v clinical.Referral) error {
	hash := referralHash(v)
	tx := blockchain.Transaction{ID: "referral-" + v.ID, Type: blockchain.ReferralCreated, ActorID: v.CreatedByActorID, OrganizationID: v.SourceOrganizationID, ResourceID: v.ID, PayloadHash: hash[:], Timestamp: v.CreatedAt}
	if _, err := s.commit(ctx, tx, v.CreatedByActorID); err != nil {
		return err
	}
	return s.Store.SetReferralCommitState(ctx, v.ID, clinical.CommitCommitted)
}
func (s *Service) RetryPendingConsent(ctx context.Context, v clinical.Consent) error {
	actorID, actor, err := s.patientConsentActor(ctx, v.PatientID)
	if err != nil {
		return err
	}
	hash := clinical.HashEvent(v.ID, v.PatientID, v.RecordID, v.GrantedToActorID, v.GrantedToOrganizationID, v.GrantedAt.Format(time.RFC3339Nano))
	tx := blockchain.Transaction{ID: "consent-grant-" + v.ID, Type: blockchain.ConsentGranted, ActorID: actorID, OrganizationID: actor.OrganizationID, ResourceID: v.ID, PayloadHash: hash[:], Timestamp: v.GrantedAt}
	if _, err := s.commit(ctx, tx, actorID); err != nil {
		return err
	}
	if err := s.Store.ActivateConsent(ctx, v.ID); err != nil {
		return err
	}
	return s.Store.SetAccessRequestStatus(ctx, v.AccessRequestID, clinical.AccessGranted)
}
func (s *Service) RetryPendingRevocation(ctx context.Context, v clinical.Consent) error {
	if v.Status != clinical.ConsentRevocationPending || v.RevokedAt == nil {
		return nil
	}
	actorID, actor, err := s.patientConsentActor(ctx, v.PatientID)
	if err != nil {
		return err
	}
	txID := "consent-revoke-" + v.ID
	if !s.Node.HasTransaction(txID) {
		hash := clinical.HashEvent(v.ID, v.PatientID, v.RecordID, v.RevokedAt.UTC().Format(time.RFC3339Nano))
		tx := blockchain.Transaction{ID: txID, Type: blockchain.ConsentRevoked, ActorID: actorID, OrganizationID: actor.OrganizationID, ResourceID: v.ID, PayloadHash: hash[:], Timestamp: v.RevokedAt.UTC()}
		if _, err := s.commit(ctx, tx, actorID); err != nil {
			return err
		}
	}
	if err := s.Store.RevokeConsent(ctx, v.ID, v.RevokedAt.UTC()); err != nil {
		return err
	}
	_ = s.Store.SetAccessRequestStatus(ctx, v.AccessRequestID, clinical.AccessRevoked)
	return nil
}
func (s *Service) CreateReferral(ctx context.Context, value clinical.Referral) (clinical.Referral, error) {
	actor, err := s.Registry.GetActor(value.CreatedByActorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != value.SourceOrganizationID || value.SourceOrganizationID != s.OrganizationID && s.OrganizationID != "" {
		return value, errors.New("invalid referral creator")
	}
	if _, err := s.Registry.GetOrganization(value.DestinationOrganizationID); err != nil {
		return value, err
	}
	record, err := s.Store.GetClinicalRecord(ctx, value.RecordID)
	if err != nil || record.PatientID != value.PatientID || record.OrganizationID != value.SourceOrganizationID || record.CommitState != clinical.CommitCommitted {
		return value, errors.New("referral record linkage invalid")
	}
	ops, ok := s.Store.(OperationalStore)
	if !ok {
		return value, errors.New("patient identity persistence unavailable")
	}
	patient, err := ops.GetPatient(ctx, value.SourceOrganizationID, value.PatientID)
	if err != nil || patient.NetworkPatientID == "" {
		return value, errors.New("referral patient identity invalid")
	}
	if value.NetworkPatientID != "" && value.NetworkPatientID != patient.NetworkPatientID {
		return value, errors.New("referral network identity mismatch")
	}
	value.NetworkPatientID = patient.NetworkPatientID
	value.Status = clinical.ReferralCreated
	value.CommitState = clinical.CommitPending
	value.CreatedAt = value.CreatedAt.UTC()
	if err := s.Store.SaveReferral(ctx, value); err != nil {
		return value, err
	}
	hash := referralHash(value)
	tx := blockchain.Transaction{ID: "referral-" + value.ID, Type: blockchain.ReferralCreated, ActorID: value.CreatedByActorID, OrganizationID: value.SourceOrganizationID, ResourceID: value.ID, PayloadHash: hash[:], Timestamp: value.CreatedAt}
	if _, err := s.commit(ctx, tx, value.CreatedByActorID); err != nil {
		return value, err
	}
	if err := s.Store.SetReferralCommitState(ctx, value.ID, clinical.CommitCommitted); err != nil {
		return value, err
	}
	value.CommitState = clinical.CommitCommitted
	if s.NodeID != "" {
		if err := s.DeliverReferral(ctx, value, time.Now().UTC()); err != nil {
			return value, err
		}
		_ = s.Store.(CrossHospitalStore).SetReferralStatus(ctx, value.ID, clinical.ReferralDelivered)
		value.Status = clinical.ReferralDelivered
	}
	s.Logger.Info("referral_created", "referral_id", value.ID)
	return value, nil
}
func (s *Service) RequestAccess(ctx context.Context, value clinical.AccessRequest) error {
	actor, err := s.Registry.GetActor(value.RequesterActorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != value.RequesterOrganizationID {
		return ErrAccessDenied
	}
	value.Status = clinical.AccessPending
	value.CreatedAt = value.CreatedAt.UTC()
	value.UpdatedAt = value.CreatedAt
	if err := s.Store.SaveAccessRequest(ctx, value); err == nil {
		s.Logger.Info("access_requested", "request_id", value.ID)
	}
	return err
}
func (s *Service) GrantConsent(ctx context.Context, requestID, consentID, patientID string, at time.Time) error {
	request, err := s.Store.GetAccessRequest(ctx, requestID)
	if err != nil {
		return err
	}
	actorID, actor, err := s.patientConsentActor(ctx, patientID)
	if err != nil || actor.Role != identity.RolePatient || request.PatientID != patientID {
		return ErrAccessDenied
	}
	consent := clinical.Consent{ID: consentID, AccessRequestID: requestID, ReferralID: request.ReferralID, PatientID: patientID, RecordID: request.RecordID, GrantedToActorID: request.RequesterActorID, GrantedToOrganizationID: request.RequesterOrganizationID, Status: clinical.ConsentPending, GrantedAt: at.UTC()}
	if err := s.Store.SaveConsent(ctx, consent); err != nil {
		return err
	}
	hash := clinical.HashEvent(consent.ID, consent.PatientID, consent.RecordID, consent.GrantedToActorID, consent.GrantedToOrganizationID, consent.GrantedAt.Format(time.RFC3339Nano))
	tx := blockchain.Transaction{ID: "consent-grant-" + consent.ID, Type: blockchain.ConsentGranted, ActorID: actorID, OrganizationID: actor.OrganizationID, ResourceID: consent.ID, PayloadHash: hash[:], Timestamp: consent.GrantedAt}
	if _, err := s.commit(ctx, tx, actorID); err != nil {
		return err
	}
	if err := s.Store.ActivateConsent(ctx, consent.ID); err != nil {
		return err
	}
	if err := s.Store.SetAccessRequestStatus(ctx, requestID, clinical.AccessGranted); err != nil {
		return err
	}
	if s.NodeID != "" {
		if err := s.DeliverAccessStatus(ctx, AccessStatusDelivery{RequestID: request.ID, ReferralID: request.ReferralID, RecordID: request.RecordID, Status: clinical.AccessGranted}, request.RequesterOrganizationID, time.Now().UTC()); err != nil {
			return err
		}
	}
	s.Logger.Info("consent_granted", "consent_id", consent.ID)
	return nil
}
func (s *Service) DeclineAccess(ctx context.Context, requestID, patientID string, at time.Time) error {
	request, err := s.Store.GetAccessRequest(ctx, requestID)
	if err != nil || request.PatientID != patientID || request.Status != clinical.AccessPending {
		return ErrAccessDenied
	}
	_, actor, err := s.patientConsentActor(ctx, patientID)
	if err != nil || actor.Role != identity.RolePatient {
		return ErrAccessDenied
	}
	if err = s.Store.SetAccessRequestStatus(ctx, requestID, clinical.AccessDenied); err != nil {
		return err
	}
	if s.NodeID != "" {
		return s.DeliverAccessStatus(ctx, AccessStatusDelivery{RequestID: request.ID, ReferralID: request.ReferralID, RecordID: request.RecordID, Status: clinical.AccessDenied}, request.RequesterOrganizationID, at.UTC())
	}
	return nil
}
func (s *Service) RevokeConsent(ctx context.Context, consentID, patientID string, at time.Time) error {
	consent, err := s.Store.GetConsent(ctx, consentID)
	if err != nil || consent.PatientID != patientID {
		return ErrAccessDenied
	}
	_, actor, err := s.patientConsentActor(ctx, patientID)
	if err != nil || actor.Role != identity.RolePatient {
		return ErrAccessDenied
	}
	if consent.Status == clinical.ConsentRevoked {
		return nil
	}
	if consent.Status == clinical.ConsentActive {
		if err := s.Store.BeginConsentRevocation(ctx, consentID, at.UTC()); err != nil {
			return err
		}
		consent.Status = clinical.ConsentRevocationPending
		revokedAt := at.UTC()
		consent.RevokedAt = &revokedAt
	} else if consent.Status != clinical.ConsentRevocationPending {
		return ErrAccessDenied
	}
	if err := s.RetryPendingRevocation(ctx, consent); err != nil {
		return err
	}
	if s.NodeID != "" {
		if request, e := s.Store.GetAccessRequest(ctx, consent.AccessRequestID); e == nil {
			if e = s.DeliverAccessStatus(ctx, AccessStatusDelivery{RequestID: request.ID, ReferralID: request.ReferralID, RecordID: request.RecordID, Status: clinical.AccessRevoked}, request.RequesterOrganizationID, time.Now().UTC()); e != nil {
				return e
			}
		}
	}
	s.Logger.Info("consent_revoked", "consent_id", consent.ID)
	return nil
}
func (s *Service) AuthorizeAndGet(ctx context.Context, recordID, requesterID, organizationID string, at time.Time) (clinical.ClinicalRecord, error) {
	actor, err := s.Registry.GetActor(requesterID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != organizationID || actor.Status != identity.ActorActive {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	record, err := s.Store.GetClinicalRecord(ctx, recordID)
	if err != nil || record.CommitState != clinical.CommitCommitted {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	if _, err := s.Store.FindActiveConsent(ctx, record.PatientID, recordID, requesterID, organizationID, at); err != nil {
		s.Logger.Warn("shared_record_access_denied", "record_id", recordID)
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	s.Logger.Info("shared_record_access_allowed", "record_id", recordID)
	return record, nil
}
func (s *Service) AuditAccess(ctx context.Context, record clinical.ClinicalRecord, requesterID, organizationID string, at time.Time) error {
	hash := clinical.HashEvent(record.ID, record.PatientID, requesterID, organizationID, at.UTC().Format(time.RFC3339Nano))
	tx := blockchain.Transaction{ID: "record-access-" + record.ID + "-" + fmt.Sprint(at.UnixNano()), Type: blockchain.RecordAccessed, ActorID: requesterID, OrganizationID: organizationID, ResourceID: record.ID, PayloadHash: hash[:], Timestamp: at.UTC()}
	_, err := s.commit(ctx, tx, requesterID)
	if err == nil {
		s.Logger.Info("record_access_audited", "record_id", record.ID)
	}
	return err
}
func (s *Service) VerifyRecord(ctx context.Context, recordID string) (bool, error) {
	record, err := s.Store.GetClinicalRecord(ctx, recordID)
	if err != nil {
		return false, err
	}
	hash := clinical.HashRecord(record)
	return string(hash[:]) == string(record.RecordHash), nil
}
