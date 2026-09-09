//go:build integration

package tamperdemo_test

import (
	"context"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/ethclient"
	workflow "github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	verify "github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/tamperdemo"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
)

const (
	authorityKey = "00000000000000000000000000000000000000000000000000000000000a11ce"
	hospitalKey  = "0000000000000000000000000000000000000000000000000000000000000b0b"
	payerKey     = "000000000000000000000000000000000000000000000000000000000000ca11"
	staffingKey  = "0000000000000000000000000000000000000000000000000000000000c0ffee"
)

var contracts = besu.ContractAddresses{
	OrganizationRegistry:   common.HexToAddress("0x1148e16d870cc191b97774d1b55f65d42ecf6cd7"),
	AuthorityRegistry:      common.HexToAddress("0xf5495ad34a7f25b26bb8b8ef512c44c7cb5cc005"),
	AuthorityEventRegistry: common.HexToAddress("0x3372e3af8edce113af8926a924f823edf2a301c2"),
	ResponseLedger:         common.HexToAddress("0xaebe3a5fc1b9049970d0a53967d07c98211bcde7"),
}

func adapter(t *testing.T, rpc, key string) *besu.Adapter {
	t.Helper()
	value, err := besu.New(context.Background(), besu.Config{
		RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: contracts,
		PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(value.Close)
	return value
}

type durableSnapshot struct {
	assertions, responses, submissions, attempts int
}

func snapshot(t *testing.T, store *authorityledger.Store) durableSnapshot {
	t.Helper()
	ctx := context.Background()
	var result durableSnapshot
	for table, destination := range map[string]*int{
		"authority_assertions": &result.assertions,
		"response_versions":    &result.responses,
		"ledger_submissions":   &result.submissions,
	} {
		value, err := store.Count(ctx, table)
		if err != nil {
			t.Fatal(err)
		}
		*destination = value
	}
	if err := store.Pool().QueryRow(ctx, `SELECT COALESCE(sum(attempt_count),0) FROM authority_ledger.ledger_submissions`).Scan(&result.attempts); err != nil {
		t.Fatal(err)
	}
	return result
}

func organizationStatus(t *testing.T, bundle verify.BundleResult, organization string) verify.VerificationStatus {
	t.Helper()
	for _, response := range bundle.Responses {
		if response.OrganizationID == organization {
			return response.Result.Status
		}
	}
	t.Fatalf("organization %s missing from bundle", organization)
	return ""
}

func TestPhase8ControlledTamperDemoPostgresBesu(t *testing.T) {
	databaseURL := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	store, err := authorityledger.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	migrator := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "..", "migrations", "authority_ledger"))
	_ = migrator.DownAll(ctx)
	if err = migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.DownAll(ctx); store.Close() })

	var address authorityledger.Address
	organizations := []authorityledger.Organization{
		{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: address},
		{ID: "PAYER-B", Type: "PAYER", DisplayName: "Payer B", Active: true, Signer: address},
		{ID: "STAFFING-AGENCY-C", Type: "STAFFING_AGENCY", DisplayName: "Staffing Agency C", Active: true, Signer: address},
	}
	for _, organization := range organizations {
		if err = store.CreateOrganization(ctx, organization); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: address}); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-7F31A", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); err != nil {
		t.Fatal(err)
	}

	authority := adapter(t, "http://127.0.0.1:8545", authorityKey)
	hospital := adapter(t, "http://127.0.0.1:8545", hospitalKey)
	payer := adapter(t, "http://127.0.0.1:8545", payerKey)
	staffing := adapter(t, "http://127.0.0.1:8545", staffingKey)
	peer := adapter(t, "http://127.0.0.1:8546", authorityKey)
	submitters := map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	reconcilers := map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	workflowService := workflow.New(store, submission.New(store, submitters), submission.NewReconciler(store, reconcilers))

	eventTime := time.Date(2026, 7, 1, 0, 0, 0, 123, time.UTC)
	source := []byte("synthetic private Phase 8 authority source")
	event, err := workflowService.RegisterAuthorityEvent(ctx, workflow.AuthorityEventInput{
		EventID: tamperdemo.FixtureEventID, SeriesID: "SERIES-PHASE8", AuthorityID: tamperdemo.FixtureAuthorityID,
		SubjectID: tamperdemo.FixtureSubjectID, OperationID: "OP-P8-EVENT", EffectiveTime: eventTime,
		SourceDocumentHash: canonical.Hash(source), SourceDocument: source,
	})
	if err != nil || event.Ledger.State != "CONFIRMED" {
		t.Fatalf("event %+v %v", event, err)
	}

	type flow struct{ organization, short, response string }
	flows := []flow{{"HOSPITAL-A", "HOSP", "P8-HOSP"}, {"PAYER-B", "PAYER", "P8-PAYER"}, {"STAFFING-AGENCY-C", "STAFF", "P8-STAFF"}}
	for index, flow := range flows {
		receiptTime := eventTime.Add(time.Duration(index+1) * time.Hour)
		start, startErr := workflowService.StartOrganizationResponse(ctx, workflow.StartResponseInput{
			ActorOrganizationID: flow.organization, OrganizationID: flow.organization, EventID: event.ID,
			ResponseID: flow.response, VersionID: flow.response + "-V1", OperationID: "OP-P8-" + flow.short + "-V1", ReceiptTime: receiptTime,
		})
		if startErr != nil || start.Ledger.State != "CONFIRMED" {
			t.Fatalf("start %s %+v %v", flow.organization, start, startErr)
		}
		for stateIndex, state := range []string{"UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"} {
			version := stateIndex + 2
			suffix := string(rune('0' + version))
			input := workflow.AdvanceResponseInput{
				ActorOrganizationID: flow.organization, OrganizationID: flow.organization, EventID: event.ID,
				VersionID: flow.response + "-V" + suffix, OperationID: "OP-P8-" + flow.short + "-V" + suffix,
				TargetState: state, OccurredAt: receiptTime.Add(time.Duration(version) * time.Hour),
				PolicyID: "POL-P8-" + flow.short, PolicyVersionID: "POL-P8-" + flow.short + "-V1", PolicyText: "private " + flow.short + " policy",
				DecisionID: "DEC-P8-" + flow.short, DecisionCode: "HOLD-" + flow.short, DecisionExplanation: "private " + flow.short + " decision", ReviewerReference: "REVIEW-" + flow.short,
				ActionID: "ACT-P8-" + flow.short, ActionCode: "COMPLETE-" + flow.short, ActionExplanation: "private " + flow.short + " action", OperatorReference: "OPERATOR-" + flow.short,
				SupportingEvidence: []byte("private " + flow.short + " support"),
			}
			advanced, advanceErr := workflowService.AdvanceOrganizationResponse(ctx, input)
			if advanceErr != nil || advanced.Ledger.State != "CONFIRMED" {
				t.Fatalf("advance %s %s %+v %v", flow.organization, state, advanced, advanceErr)
			}
		}
	}

	verifier := verify.New(store, authority)
	clean, err := verifier.VerifyEventBundle(ctx, event.ID)
	if err != nil || clean.Status != verify.StatusVerified {
		t.Fatalf("clean bundle %+v %v", clean, err)
	}
	beforeDisabled := snapshot(t, store)
	t.Setenv(tamperdemo.EnvironmentVariable, "false")
	disabled := tamperdemo.New(store, store.Pool(), verifier)
	for _, target := range []tamperdemo.Target{tamperdemo.TargetAuthorityEvent, tamperdemo.TargetHospitalAPolicy} {
		if _, disabledErr := disabled.Demonstrate(ctx, target); !errors.Is(disabledErr, tamperdemo.ErrDemoTamperDisabled) {
			t.Fatalf("disabled target %s: %v", target, disabledErr)
		}
	}
	if afterDisabled := snapshot(t, store); afterDisabled != beforeDisabled {
		t.Fatalf("disabled service changed durable state: before=%+v after=%+v", beforeDisabled, afterDisabled)
	}
	if stillClean, cleanErr := verifier.VerifyEventBundle(ctx, event.ID); cleanErr != nil || stillClean.Status != verify.StatusVerified {
		t.Fatalf("disabled service changed verification: %+v %v", stillClean, cleanErr)
	}

	hospitalChainBefore, err := authority.GetLatestResponse(ctx, ledger.MustID(event.ID), ledger.MustID("HOSPITAL-A"))
	if err != nil {
		t.Fatal(err)
	}
	hospitalSubmissionBefore, err := store.GetSubmissionByDomain(ctx, submission.OperationResponseVersion, hospitalChainBefore.ResponseVersionID.String())
	if err != nil {
		t.Fatal(err)
	}
	rpc, err := ethclient.Dial("http://127.0.0.1:8545")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(rpc.Close)
	blockBefore, err := rpc.BlockNumber(ctx)
	if err != nil {
		t.Fatal(err)
	}
	durableBefore := snapshot(t, store)

	t.Setenv(tamperdemo.EnvironmentVariable, "true")
	demo := tamperdemo.New(store, store.Pool(), verifier)
	hospitalResult, err := demo.Demonstrate(ctx, tamperdemo.TargetHospitalAPolicy)
	if err != nil {
		t.Fatal(err)
	}
	if hospitalResult.Before.Status != verify.StatusVerified || hospitalResult.After.Status != verify.StatusFailed || hospitalResult.Before.LocalCommitment != hospitalResult.Before.LedgerCommitment || hospitalResult.After.LocalCommitment == hospitalResult.After.LedgerCommitment || !hospitalResult.LedgerUnchanged {
		t.Fatalf("hospital result %+v", hospitalResult)
	}
	afterHospital, err := verifier.VerifyEventBundle(ctx, event.ID)
	if err != nil || afterHospital.Status != verify.StatusFailed || afterHospital.Authority.Status != verify.StatusVerified || organizationStatus(t, afterHospital, "HOSPITAL-A") != verify.StatusFailed || organizationStatus(t, afterHospital, "PAYER-B") != verify.StatusVerified || organizationStatus(t, afterHospital, "STAFFING-AGENCY-C") != verify.StatusVerified {
		t.Fatalf("hospital isolation bundle %+v %v", afterHospital, err)
	}
	hospitalChainAfter, err := authority.GetLatestResponse(ctx, ledger.MustID(event.ID), ledger.MustID("HOSPITAL-A"))
	if err != nil || hospitalChainAfter != hospitalChainBefore {
		t.Fatalf("primary ledger response changed: before=%+v after=%+v err=%v", hospitalChainBefore, hospitalChainAfter, err)
	}
	hospitalPeerAfter, err := peer.GetLatestResponse(ctx, ledger.MustID(event.ID), ledger.MustID("HOSPITAL-A"))
	if err != nil || hospitalPeerAfter != hospitalChainBefore {
		t.Fatalf("peer ledger response changed: before=%+v peer=%+v err=%v", hospitalChainBefore, hospitalPeerAfter, err)
	}
	hospitalSubmissionAfter, err := store.GetSubmissionByDomain(ctx, submission.OperationResponseVersion, hospitalChainBefore.ResponseVersionID.String())
	if err != nil || hospitalSubmissionAfter.OperationID != hospitalSubmissionBefore.OperationID || string(hospitalSubmissionAfter.TransactionHash) != string(hospitalSubmissionBefore.TransactionHash) {
		t.Fatalf("hospital transaction identity changed: before=%+v after=%+v err=%v", hospitalSubmissionBefore, hospitalSubmissionAfter, err)
	}
	if _, repeatErr := demo.Demonstrate(ctx, tamperdemo.TargetHospitalAPolicy); !errors.Is(repeatErr, tamperdemo.ErrPrecondition) {
		t.Fatalf("repeated hospital tamper: %v", repeatErr)
	}

	authorityChainBefore, err := authority.GetAuthorityAssertion(ctx, ledger.MustID(event.ID))
	if err != nil {
		t.Fatal(err)
	}
	authoritySubmissionBefore, err := store.GetSubmissionByDomain(ctx, submission.OperationAuthorityAssertion, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	authorityResult, err := demo.Demonstrate(ctx, tamperdemo.TargetAuthorityEvent)
	if err != nil {
		t.Fatal(err)
	}
	if authorityResult.Before.Status != verify.StatusVerified || authorityResult.After.Status != verify.StatusFailed || authorityResult.Before.LocalCommitment != authorityResult.Before.LedgerCommitment || authorityResult.After.LocalCommitment == authorityResult.After.LedgerCommitment || !authorityResult.LedgerUnchanged {
		t.Fatalf("authority result %+v", authorityResult)
	}
	authorityChainAfter, err := authority.GetAuthorityAssertion(ctx, ledger.MustID(event.ID))
	if err != nil || authorityChainAfter != authorityChainBefore {
		t.Fatalf("primary authority ledger changed: before=%+v after=%+v err=%v", authorityChainBefore, authorityChainAfter, err)
	}
	authorityPeerAfter, err := peer.GetAuthorityAssertion(ctx, ledger.MustID(event.ID))
	if err != nil || authorityPeerAfter != authorityChainBefore {
		t.Fatalf("peer authority ledger changed: before=%+v peer=%+v err=%v", authorityChainBefore, authorityPeerAfter, err)
	}
	authoritySubmissionAfter, err := store.GetSubmissionByDomain(ctx, submission.OperationAuthorityAssertion, event.ID)
	if err != nil || authoritySubmissionAfter.OperationID != authoritySubmissionBefore.OperationID || string(authoritySubmissionAfter.TransactionHash) != string(authoritySubmissionBefore.TransactionHash) {
		t.Fatalf("authority transaction identity changed: before=%+v after=%+v err=%v", authoritySubmissionBefore, authoritySubmissionAfter, err)
	}
	if _, repeatErr := demo.Demonstrate(ctx, tamperdemo.TargetAuthorityEvent); !errors.Is(repeatErr, tamperdemo.ErrPrecondition) {
		t.Fatalf("repeated authority tamper: %v", repeatErr)
	}

	if durableAfter := snapshot(t, store); durableAfter != durableBefore {
		t.Fatalf("demo created workflow/submission side effects: before=%+v after=%+v", durableBefore, durableAfter)
	}
	blockAfter, err := rpc.BlockNumber(ctx)
	if err != nil || blockAfter != blockBefore {
		t.Fatalf("demo caused unexpected Besu block activity: before=%d after=%d err=%v", blockBefore, blockAfter, err)
	}
}
