package workflow_test

import (
	"context"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
	pg "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/recovery"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/workflow"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"
)

func TestRealNormalVisitPersistsAcrossRestart(t *testing.T) {
	url := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL_B")
	if url == "" {
		t.Skip("real PostgreSQL B URL required")
	}
	ctx := context.Background()
	store, err := pg.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	pool, _ := pgxpool.New(ctx, url)
	_, err = pool.Exec(ctx, `TRUNCATE visits,patients,outgoing_access_requests,incoming_referrals,workflow_reconciliation,consumed_peer_requests,consumed_requester_proofs,idempotency_keys,consents,access_requests,referrals,clinical_records,transactions,blocks,actors,organizations CASCADE`)
	pool.Close()
	if err != nil {
		t.Fatal(err)
	}
	policy := authorization.HealthcarePolicy{}
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state, err := recovery.Load(ctx, store, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	n := node.New("hospital-b-node", "hospital-b", state.Chain, state.Registry, policy, nil, nil, logger)
	n.SetBlockStore(store)
	_, keys, _ := demodata.Registry()
	service := workflow.Service{Store: store, Node: n, Registry: state.Registry, Keys: keys, Logger: logger, OrganizationID: "hospital-b"}
	patient := clinical.Patient{ID: "patient-x", FullName: "Patient X", DateOfBirth: "1990-02-03", Gender: "Female", Phone: "555-0100"}
	if _, err = service.RegisterPatient(ctx, patient); err != nil {
		t.Fatal(err)
	}
	if _, err = service.RegisterPatient(ctx, patient); err == nil {
		t.Fatal("duplicate patient accepted")
	}
	visit := clinical.Visit{ID: "visit-x", PatientID: "patient-x", DoctorID: "doctor-b", Room: "204", Reason: "Normal consultation", Source: clinical.VisitNormal}
	visit, err = service.CheckIn(ctx, visit)
	if err != nil || visit.Status != clinical.VisitWaiting {
		t.Fatalf("check-in: %v %+v", err, visit)
	}
	queue, err := store.ListVisits(ctx, "hospital-b", "doctor-b")
	if err != nil || len(queue) != 1 {
		t.Fatal("doctor queue missing check-in")
	}
	now := genesis.Add(time.Hour)
	if err = service.StartVisit(ctx, visit.ID, "doctor-b", now); err != nil {
		t.Fatal(err)
	}
	draft := clinical.Visit{ChiefComplaint: "Headache", ClinicalNotes: "Exam completed", Diagnosis: "Tension headache", Prescription: "Rest and hydration", FollowUpPlan: "Return if symptoms persist"}
	if err = service.SaveVisitDraft(ctx, visit.ID, "doctor-b", draft); err != nil {
		t.Fatal(err)
	}
	if n.HasTransaction("record-commit-record-x") {
		t.Fatal("draft committed a blockchain record")
	}
	record, err := service.CompleteVisit(ctx, visit.ID, "doctor-b", "record-x", now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if record.OrganizationID != "hospital-b" || !n.HasTransaction("record-commit-record-x") {
		t.Fatal("completion did not create Hospital B record commitment")
	}
	loaded, err := store.GetVisit(ctx, visit.ID)
	if err != nil || loaded.Status != clinical.VisitCompleted || loaded.RecordID != record.ID {
		t.Fatal("visit completion not persisted")
	}
	store.Close()
	reopened, err := pg.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	recovered, err := reopened.GetVisit(ctx, visit.ID)
	if err != nil || recovered.Status != clinical.VisitCompleted {
		t.Fatal("completed visit missing after restart")
	}
}
