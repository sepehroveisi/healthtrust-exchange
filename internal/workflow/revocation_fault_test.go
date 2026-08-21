package workflow

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/reliability"
)

type revocationFaultStore struct {
	Store
	consent     clinical.Consent
	request     clinical.AccessRequest
	record      clinical.ClinicalRecord
	beginErr    error
	finalizeErr error
	idemHash    [sha256.Size]byte
	idemStatus  string
}

func (s *revocationFaultStore) GetConsent(context.Context, string) (clinical.Consent, error) {
	return s.consent, nil
}
func (s *revocationFaultStore) GetAccessRequest(context.Context, string) (clinical.AccessRequest, error) {
	return s.request, nil
}
func (s *revocationFaultStore) BeginIdempotent(_ context.Context, _, _ string, _ string, hash [sha256.Size]byte) (reliability.IdempotencyResult, error) {
	if s.idemStatus == "" {
		s.idemHash, s.idemStatus = hash, "Pending"
		return reliability.IdempotencyResult{Owner: true}, nil
	}
	if s.idemHash != hash {
		return reliability.IdempotencyResult{}, reliability.ErrIdempotencyConflict
	}
	if s.idemStatus == "Pending" {
		return reliability.IdempotencyResult{}, reliability.ErrOperationPending
	}
	return reliability.IdempotencyResult{ResponseReference: s.consent.ID}, nil
}
func (s *revocationFaultStore) CompleteIdempotent(context.Context, string, string, string) error {
	if s.idemStatus != "Pending" {
		return errors.New("not pending")
	}
	s.idemStatus = "Completed"
	return nil
}
func (s *revocationFaultStore) BeginConsentRevocation(_ context.Context, _ string, at time.Time) error {
	if s.beginErr != nil {
		return s.beginErr
	}
	if s.consent.Status != clinical.ConsentActive {
		return errors.New("not active")
	}
	s.consent.Status = clinical.ConsentRevocationPending
	s.consent.RevokedAt = pointerTo(at.UTC())
	return nil
}
func (s *revocationFaultStore) RevokeConsent(_ context.Context, _ string, at time.Time) error {
	if s.finalizeErr != nil {
		return s.finalizeErr
	}
	s.consent.Status = clinical.ConsentRevoked
	s.consent.RevokedAt = pointerTo(at.UTC())
	return nil
}
func (s *revocationFaultStore) SetAccessRequestStatus(_ context.Context, _ string, status clinical.AccessStatus) error {
	s.request.Status = status
	return nil
}
func (s *revocationFaultStore) GetClinicalRecord(context.Context, string) (clinical.ClinicalRecord, error) {
	return s.record, nil
}
func (s *revocationFaultStore) FindActiveConsent(_ context.Context, patient, record, actor, organization string, _ time.Time) (clinical.Consent, error) {
	v := s.consent
	if v.Status != clinical.ConsentActive || v.PatientID != patient || v.RecordID != record || v.GrantedToActorID != actor || v.GrantedToOrganizationID != organization {
		return clinical.Consent{}, errors.New("active consent not found")
	}
	return v, nil
}

type revocationFaultNode struct {
	submitted []blockchain.Transaction
	committed map[string]bool
	commitErr error
}

func (n *revocationFaultNode) SubmitTransaction(tx blockchain.Transaction) error {
	n.submitted = append(n.submitted, tx)
	return nil
}
func (n *revocationFaultNode) CreateBlock(context.Context, time.Time) (blockchain.Block, error) {
	if n.commitErr != nil {
		return blockchain.Block{}, n.commitErr
	}
	tx := n.submitted[len(n.submitted)-1]
	n.committed[tx.ID] = true
	return blockchain.Block{Height: uint64(len(n.committed))}, nil
}
func (n *revocationFaultNode) HasTransaction(id string) bool { return n.committed[id] }

func pointerTo(value time.Time) *time.Time { return &value }

func revocationFixture(t *testing.T) (*Service, *revocationFaultStore, *revocationFaultNode, time.Time) {
	t.Helper()
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	patientKey := keyFor("patient-a")
	doctorKey := keyFor("doctor-b")
	registry := identity.NewMemoryRegistry()
	if err := registry.RegisterOrganization(identity.Organization{ID: "hospital-a", Name: "Hospital A", Status: identity.OrganizationActive}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterOrganization(identity.Organization{ID: "hospital-b", Name: "Hospital B", Status: identity.OrganizationActive}); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []identity.Actor{
		{ID: "patient-a", OrganizationID: "hospital-a", Role: identity.RolePatient, PublicKey: patientKey.Public().(ed25519.PublicKey), Status: identity.ActorActive},
		{ID: "doctor-b", OrganizationID: "hospital-b", Role: identity.RoleDoctor, PublicKey: doctorKey.Public().(ed25519.PublicKey), Status: identity.ActorActive},
	} {
		if err := registry.RegisterActor(actor); err != nil {
			t.Fatal(err)
		}
	}
	store := &revocationFaultStore{
		consent: clinical.Consent{ID: "consent-1", AccessRequestID: "request-1", PatientID: "patient-a", RecordID: "record-a", GrantedToActorID: "doctor-b", GrantedToOrganizationID: "hospital-b", Status: clinical.ConsentActive, GrantedAt: now.Add(-time.Hour)},
		request: clinical.AccessRequest{ID: "request-1", PatientID: "patient-a", RecordID: "record-a", RequesterActorID: "doctor-b", RequesterOrganizationID: "hospital-b", Status: clinical.AccessGranted},
		record:  clinical.ClinicalRecord{ID: "record-a", PatientID: "patient-a", OrganizationID: "hospital-a", CommitState: clinical.CommitCommitted},
	}
	node := &revocationFaultNode{committed: map[string]bool{}}
	service := &Service{Store: store, Node: node, Registry: registry, Keys: map[string]ed25519.PrivateKey{"patient-a": patientKey, "doctor-b": doctorKey}, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	return service, store, node, now
}

func keyFor(label string) ed25519.PrivateKey {
	seed := sha256.Sum256([]byte(label))
	return ed25519.NewKeyFromSeed(seed[:])
}

func assertSourceAccess(t *testing.T, service *Service, now time.Time, allowed bool) {
	t.Helper()
	_, err := service.AuthorizeAndGet(context.Background(), "record-a", "doctor-b", "hospital-b", now)
	if allowed && err != nil {
		t.Fatalf("source-side authorization unexpectedly denied: %v", err)
	}
	if !allowed && !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("source-side authorization did not fail closed: %v", err)
	}
}

func TestRevocationFailureBeforeFailClosedTransitionCreatesNoEvidence(t *testing.T) {
	service, store, node, now := revocationFixture(t)
	store.beginErr = errors.New("database unavailable")
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now); err == nil {
		t.Fatal("revocation unexpectedly succeeded")
	}
	if store.consent.Status != clinical.ConsentActive || len(node.submitted) != 0 {
		t.Fatal("failure before transition changed authorization or created evidence")
	}
	assertSourceAccess(t, service, now, true)
}

func TestRevocationBlockchainFailureRemainsFailClosed(t *testing.T) {
	service, store, node, now := revocationFixture(t)
	node.commitErr = errors.New("block creation failed")
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now); err == nil {
		t.Fatal("revocation unexpectedly succeeded")
	}
	if store.consent.Status != clinical.ConsentRevocationPending || node.HasTransaction("consent-revoke-consent-1") {
		t.Fatal("failed blockchain commit did not leave a recoverable fail-closed state")
	}
	assertSourceAccess(t, service, now, false)
}

func TestRevocationFinalizationFailureRemainsFailClosedAndRecovers(t *testing.T) {
	service, store, node, now := revocationFixture(t)
	store.finalizeErr = errors.New("database finalization failed")
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now); err == nil {
		t.Fatal("revocation unexpectedly succeeded")
	}
	if store.consent.Status != clinical.ConsentRevocationPending || !node.HasTransaction("consent-revoke-consent-1") {
		t.Fatal("blockchain success did not retain fail-closed pending state")
	}
	assertSourceAccess(t, service, now, false)
	submissions := len(node.submitted)
	store.finalizeErr = nil
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now.Add(time.Minute)); err != nil {
		t.Fatalf("retry did not reconcile revocation: %v", err)
	}
	if store.consent.Status != clinical.ConsentRevoked || len(node.submitted) != submissions {
		t.Fatal("retry did not finalize without duplicate blockchain evidence")
	}
	assertSourceAccess(t, service, now, false)
}

func TestRepeatedRevocationIsSafeAndIdempotent(t *testing.T) {
	service, store, node, now := revocationFixture(t)
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now); err != nil {
		t.Fatal(err)
	}
	submissions := len(node.submitted)
	if err := service.RevokeConsent(context.Background(), "consent-1", "patient-a", now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if store.consent.Status != clinical.ConsentRevoked || len(node.submitted) != submissions {
		t.Fatal("repeated revoke changed authorization or duplicated evidence")
	}
	assertSourceAccess(t, service, now, false)
}

func TestSynchronizationFailureCannotRestoreSourceAuthorizationAndRetryCompletes(t *testing.T) {
	service, store, node, now := revocationFixture(t)
	service.NodeID = "hospital-a-node"
	service.OrganizationID = "hospital-a"
	service.PeerURLs = map[string]string{}
	key := "revoke-timeout"
	if err := service.RevokeConsentIdempotent(context.Background(), key, "consent-1", "patient-a", now); err == nil {
		t.Fatal("missing peer unexpectedly reported synchronization success")
	}
	if store.consent.Status != clinical.ConsentRevoked || store.idemStatus != "Pending" {
		t.Fatal("source revocation or retry marker was not preserved")
	}
	assertSourceAccess(t, service, now, false)
	submissions := len(node.submitted)
	if err := service.RevokeConsentIdempotent(context.Background(), key, "consent-1", "patient-a", now.Add(time.Minute)); err != nil {
		t.Fatalf("timeout retry did not complete idempotently: %v", err)
	}
	if store.idemStatus != "Completed" || len(node.submitted) != submissions {
		t.Fatal("retry duplicated blockchain evidence or remained pending")
	}
	assertSourceAccess(t, service, now, false)
}
