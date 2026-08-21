package workflow_test

import (
	"context"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
	pg "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/recovery"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/reliability"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

type directTransport struct{ target *node.Node }

func (d directTransport) BroadcastBlock(ctx context.Context, _ []node.Peer, b blockchain.Block) error {
	return d.target.ReceiveBlockContext(ctx, b)
}
func (d directTransport) FetchBlocks(context.Context, node.Peer, uint64) ([]blockchain.Block, error) {
	return nil, nil
}
func TestRealTwoHospitalWorkflow(t *testing.T) {
	aURL, bURL := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL"), os.Getenv("HEALTHTRUST_TEST_DATABASE_URL_B")
	if aURL == "" || bURL == "" {
		t.Skip("real PostgreSQL URLs required")
	}
	ctx := context.Background()
	a, err := pg.Open(ctx, aURL)
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	b, err := pg.Open(ctx, bURL)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	for _, s := range []*pg.Store{a, b} {
		if err := s.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, url := range []string{aURL, bURL} {
		pool, _ := pgxpool.New(ctx, url)
		_, err := pool.Exec(ctx, `TRUNCATE visits,patients,outgoing_access_requests,incoming_referrals,workflow_reconciliation,consumed_peer_requests,consumed_requester_proofs,idempotency_keys,consents,access_requests,referrals,clinical_records,transactions,blocks,actors,organizations CASCADE`)
		pool.Close()
		if err != nil {
			t.Fatal(err)
		}
	}
	policy := authorization.HealthcarePolicy{}
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stateA, err := recovery.Load(ctx, a, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	stateB, err := recovery.Load(ctx, b, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nB := node.New("b", "hospital-b", stateB.Chain, stateB.Registry, policy, nil, nil, logger)
	nB.SetBlockStore(b)
	nA := node.New("a", "hospital-a", stateA.Chain, stateA.Registry, policy, nil, directTransport{nB}, logger)
	nA.SetBlockStore(a)
	_, keys, _ := demodata.Registry()
	service := workflow.Service{Store: a, Node: nA, Registry: stateA.Registry, Keys: keys, Logger: logger}
	now := genesis.Add(time.Hour)
	requestRecord := clinical.ClinicalRecord{ID: "record-001", PatientID: "patient-p", AuthorDoctorID: "doctor-a", OrganizationID: "hospital-a", EncounterSummary: "fictional encounter", Diagnosis: "fictional diagnosis", Prescription: "fictional prescription", CreatedAt: now}
	record, err := service.CreateClinicalRecordIdempotent(ctx, "K1", requestRecord)
	if err != nil {
		t.Fatal(err)
	}
	if record.CommitState != clinical.CommitCommitted {
		t.Fatal("record not committed")
	}
	heightAfterRecord, _ := nA.Chain.GetLatestBlock()
	retried, err := service.CreateClinicalRecordIdempotent(ctx, "K1", requestRecord)
	if err != nil || retried.ID != record.ID {
		t.Fatal("idempotent retry failed")
	}
	heightAfterRetry, _ := nA.Chain.GetLatestBlock()
	if heightAfterRetry.Height != heightAfterRecord.Height {
		t.Fatal("idempotent retry created duplicate event")
	}
	changed := requestRecord
	changed.Diagnosis = "different"
	if _, err := service.CreateClinicalRecordIdempotent(ctx, "K1", changed); !errors.Is(err, reliability.ErrIdempotencyConflict) {
		t.Fatal("idempotency key reuse accepted different payload")
	}
	if _, err := b.GetClinicalRecord(ctx, record.ID); err == nil {
		t.Fatal("record duplicated to Hospital B")
	}
	_, err = service.CreateReferralIdempotent(ctx, "K2", clinical.Referral{ID: "ref-001", PatientID: "patient-p", RecordID: record.ID, SourceOrganizationID: "hospital-a", DestinationOrganizationID: "hospital-b", CreatedByActorID: "doctor-a", CreatedAt: now.Add(time.Minute)})
	if err != nil {
		t.Fatal(err)
	}
	request := clinical.AccessRequest{ID: "req-001", ReferralID: "ref-001", PatientID: "patient-p", RecordID: record.ID, RequesterActorID: "doctor-b", RequesterOrganizationID: "hospital-b", CreatedAt: now.Add(2 * time.Minute)}
	if err := service.RequestAccess(ctx, request); err != nil {
		t.Fatal(err)
	}
	if err := service.GrantConsent(ctx, request.ID, "doctor-consent", "doctor-a", now.Add(3*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
		t.Fatal("doctor granted patient consent")
	}
	proof := workflow.SignScopedRequesterProof("doctor-b", "hospital-b", record.ID, "ref-001", request.ID, now.Add(3*time.Minute), keys["doctor-b"])
	if _, err := service.AuthorizeProofAndGet(ctx, proof, now.Add(3*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
		t.Fatalf("access before consent = %v", err)
	}
	forged := workflow.SignScopedRequesterProof("doctor-b", "hospital-b", record.ID, "ref-001", request.ID, now.Add(3*time.Minute), keys["doctor-a"])
	if _, err := service.AuthorizeProofAndGet(ctx, forged, now.Add(3*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
		t.Fatal("forged requester identity accepted")
	}
	if err := service.GrantConsentIdempotent(ctx, "K3", request.ID, "consent-001", "patient-p", now.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	shared, err := service.AuthorizeProofAndGet(ctx, proof, now.Add(3*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeProofAndGet(ctx, proof, now.Add(3*time.Minute)); !errors.Is(err, security.ErrReplayedRequest) {
		t.Fatalf("requester proof replay accepted: %v", err)
	}
	if err := service.AuditAccessIdempotent(ctx, "K4", shared, "doctor-b", "hospital-b", now.Add(4*time.Minute)); err != nil {
		t.Fatal(err)
	}
	for _, mismatch := range []struct{ record, actor, organization string }{{"another-record", "doctor-b", "hospital-b"}, {record.ID, "doctor-a", "hospital-b"}, {record.ID, "doctor-b", "hospital-a"}} {
		if _, err := service.AuthorizeAndGet(ctx, mismatch.record, mismatch.actor, mismatch.organization, now.Add(4*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
			t.Fatal("mismatched consent granted access")
		}
	}
	valid, err := service.VerifyRecord(ctx, record.ID)
	if err != nil || !valid {
		t.Fatal("integrity should be valid")
	}
	pool, _ := pgxpool.New(ctx, aURL)
	_, _ = pool.Exec(ctx, `UPDATE clinical_records SET diagnosis='tampered' WHERE id='record-001'`)
	pool.Close()
	valid, _ = service.VerifyRecord(ctx, record.ID)
	if valid {
		t.Fatal("tampering not detected")
	}
	// Restore through rollback-equivalent direct correction for remaining consent checks.
	pool, _ = pgxpool.New(ctx, aURL)
	_, _ = pool.Exec(ctx, `UPDATE clinical_records SET diagnosis='fictional diagnosis' WHERE id='record-001'`)
	pool.Close()
	if err := service.RevokeConsentIdempotent(ctx, "K5", "consent-001", "patient-p", now.Add(5*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.AuthorizeProofAndGet(ctx, proof, now.Add(5*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
		t.Fatal("access after revocation allowed")
	}
	recoveredA, err := recovery.Load(ctx, a, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	recoveredRecord, err := a.GetClinicalRecord(ctx, record.ID)
	if err != nil || recoveredRecord.ID != record.ID {
		t.Fatal("record missing after restart recovery")
	}
	recoveredConsent, err := a.GetConsent(ctx, "consent-001")
	if err != nil || recoveredConsent.Status != clinical.ConsentRevoked {
		t.Fatal("revoked consent missing after restart recovery")
	}
	restartedService := workflow.Service{Store: a, Node: nA, Registry: recoveredA.Registry, Keys: keys, Logger: logger}
	if _, err := restartedService.AuthorizeProofAndGet(ctx, proof, now.Add(5*time.Minute)); !errors.Is(err, workflow.ErrAccessDenied) {
		t.Fatal("revoked access allowed after restart")
	}
	latestA, _ := nA.Chain.GetLatestBlock()
	latestB, _ := nB.Chain.GetLatestBlock()
	if latestA.Hash != latestB.Hash {
		t.Fatal("chains did not converge")
	}
	seen := map[blockchain.TransactionType]bool{}
	for _, block := range nA.Chain.Blocks() {
		for _, tx := range block.Transactions {
			seen[tx.Type] = true
		}
	}
	for _, event := range []blockchain.TransactionType{blockchain.RecordCommitted, blockchain.ReferralCreated, blockchain.ConsentGranted, blockchain.RecordAccessed, blockchain.ConsentRevoked} {
		if !seen[event] {
			t.Fatalf("missing blockchain event %s", event)
		}
	}
	t.Log("Access before consent: denied")
	t.Log("Consent granted: true")
	t.Log("Access after consent: allowed")
	t.Log("Integrity valid: true; tampering detected: true")
	t.Logf("Hospital A height: %d; Hospital B height: %d; chains equal: true", latestA.Height, latestB.Height)
	t.Log("Consent revoked: true; access after revocation: denied")
}
