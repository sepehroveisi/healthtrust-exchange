//go:build integration

package submission

import (
	"context"
	"errors"
	"math/big"
	"path/filepath"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

const (
	phase5DPayerKey    = "000000000000000000000000000000000000000000000000000000000000ca11"
	phase5DStaffingKey = "0000000000000000000000000000000000000000000000000000000000c0ffee"
)

type phase5DVersion struct {
	local      authorityledger.ResponseVersion
	chain      ledger.ResponseVersion
	submission authorityledger.Submission
}
type phase5DFlow struct {
	org               authorityledger.OrganizationID
	short, responseID string
	receipt           time.Time
	policy            *authorityledger.Policy
	decision          *authorityledger.Decision
	action            *authorityledger.Action
	versions          []phase5DVersion
}

func phase5DAdapter(t *testing.T, rpc, key string) *besu.Adapter {
	t.Helper()
	value, err := besu.New(context.Background(), besu.Config{RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: testContracts, PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func phase5DOpen(t *testing.T, ctx context.Context, url string) *authorityledger.Store {
	t.Helper()
	value, err := authorityledger.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
func phase5DSubmission(id, kind, domain, action, signer string, c authorityledger.Commitment) authorityledger.Submission {
	return authorityledger.Submission{OperationID: authorityledger.OperationID(id), OperationType: kind, DomainVersionID: domain, ContractAction: action, SignerContext: signer, IntendedCommitment: c, State: "LOCAL_PENDING"}
}
func phase5DCanonicalCommitment(t *testing.T, value canonical.Commitment) authorityledger.Commitment {
	t.Helper()
	return commitment(value)
}

func (f *phase5DFlow) append(t *testing.T, ctx context.Context, store *authorityledger.Store, event authorityledger.AssertionID, stage int) phase5DVersion {
	t.Helper()
	versionID := f.responseID + "-V" + string(rune('0'+stage))
	var previous *authorityledger.ResponseVersionID
	if stage > 1 {
		x := f.versions[len(f.versions)-1].local.ID
		previous = &x
	}
	if stage == 2 {
		canonicalPolicy := canonical.PolicyV1{PolicyID: "POL-" + f.short, OrganizationID: string(f.org), PolicyVersion: "POL-" + f.short + "-V1", EffectiveFrom: f.receipt, PolicyText: "private " + f.short + " policy"}
		hash, err := canonical.PolicyCommitment(canonicalPolicy)
		if err != nil {
			t.Fatal(err)
		}
		p := authorityledger.Policy{ID: canonicalPolicy.PolicyID, VersionID: canonicalPolicy.PolicyVersion, OrganizationID: f.org, EffectiveFromUnixNano: f.receipt.UnixNano(), Text: canonicalPolicy.PolicyText, Commitment: phase5DCanonicalCommitment(t, hash), CanonicalVersion: 1}
		if err = store.CreatePolicy(ctx, p); err != nil {
			t.Fatal(err)
		}
		f.policy = &p
	}
	if stage == 3 {
		value := canonical.DecisionV1{DecisionID: "DEC-" + f.short, EventID: string(event), OrganizationID: string(f.org), PolicyVersionHash: canonical.Commitment(f.policy.Commitment), DecisionCode: "HOLD-" + f.short, DecisionExplanation: "private independent decision", DecidedAt: f.receipt.Add(time.Hour), ReviewerReference: "REVIEW-" + f.short}
		hash, err := canonical.DecisionCommitment(value)
		if err != nil {
			t.Fatal(err)
		}
		d := authorityledger.Decision{ID: value.DecisionID, EventID: event, OrganizationID: f.org, PolicyVersionID: f.policy.VersionID, Code: value.DecisionCode, Explanation: value.DecisionExplanation, ReviewerReference: value.ReviewerReference, DecidedUnixNano: value.DecidedAt.UnixNano(), Commitment: phase5DCanonicalCommitment(t, hash), CanonicalVersion: 1}
		if err = store.CreateDecision(ctx, d); err != nil {
			t.Fatal(err)
		}
		f.decision = &d
	}
	if stage == 4 {
		evidence := canonical.Hash([]byte("private " + f.short + " support"))
		value := canonical.ActionV1{ActionID: "ACT-" + f.short, EventID: string(event), OrganizationID: string(f.org), DecisionHash: canonical.Commitment(f.decision.Commitment), ActionCode: "COMPLETE-" + f.short, ActionExplanation: "private independent action", SupportingEvidenceHash: &evidence, CompletedAt: f.receipt.Add(2 * time.Hour), OperatorReference: "OPERATE-" + f.short}
		hash, err := canonical.ActionCommitment(value)
		if err != nil {
			t.Fatal(err)
		}
		a := authorityledger.Action{ID: value.ActionID, EventID: event, OrganizationID: f.org, DecisionID: f.decision.ID, Code: value.ActionCode, Explanation: value.ActionExplanation, OperatorReference: value.OperatorReference, CompletedUnixNano: value.CompletedAt.UnixNano(), SupportingEvidence: []byte("private " + f.short + " support"), Commitment: phase5DCanonicalCommitment(t, hash), CanonicalVersion: 1}
		if err = store.CreateAction(ctx, a); err != nil {
			t.Fatal(err)
		}
		f.action = &a
	}
	state := []string{"", "RECEIVED", "UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"}[stage]
	canonicalState := canonical.ResponseState(stage)
	canonicalValue := canonical.ResponseV1{ResponseID: f.responseID, ResponseVersionID: versionID, EventID: string(event), OrganizationID: string(f.org), ResponseState: canonicalState, ReceiptTimestamp: f.receipt}
	local := authorityledger.ResponseVersion{ID: authorityledger.ResponseVersionID(versionID), ResponseID: authorityledger.ResponseID(f.responseID), EventID: event, OrganizationID: f.org, PreviousID: previous, State: state, ReceiptUnixNano: f.receipt.UnixNano(), CanonicalVersion: 1}
	if f.policy != nil {
		x := f.policy.VersionID
		local.PolicyVersionID = &x
		h := canonical.Commitment(f.policy.Commitment)
		canonicalValue.PolicyVersionHash = &h
	}
	if f.decision != nil {
		x := f.decision.ID
		local.DecisionID = &x
		h := canonical.Commitment(f.decision.Commitment)
		canonicalValue.DecisionHash = &h
	}
	if f.action != nil {
		x := f.action.ID
		local.ActionID = &x
		h := canonical.Commitment(f.action.Commitment)
		canonicalValue.ActionHash = &h
	}
	hash, err := canonical.ResponseCommitment(canonicalValue)
	if err != nil {
		t.Fatal(err)
	}
	local.Commitment = phase5DCanonicalCommitment(t, hash)
	sub := phase5DSubmission("OP-"+f.short+"-V"+string(rune('0'+stage)), OperationResponseVersion, versionID, "recordResponseVersion", string(f.org), local.Commitment)
	if err = store.CreateResponseWithPending(ctx, local, sub); err != nil {
		t.Fatal(err)
	}
	var responseID, responseVersionID, eventID, orgID ledger.ID
	responseID = ledger.MustID(f.responseID)
	responseVersionID = ledger.MustID(versionID)
	eventID = ledger.MustID(string(event))
	orgID = ledger.MustID(string(f.org))
	var predecessor ledger.ID
	if previous != nil {
		predecessor = ledger.MustID(string(*previous))
	}
	chain := ledger.ResponseVersion{ResponseID: responseID, ResponseVersionID: responseVersionID, PreviousResponseVersionID: predecessor, EventID: eventID, OrganizationID: orgID, State: ledger.ResponseState(stage), ReceiptTimestamp: f.receipt.UnixNano()}
	if f.policy != nil {
		chain.PolicyVersionHash = ledger.Commitment(f.policy.Commitment)
	}
	if f.decision != nil {
		chain.DecisionCommitment = ledger.Commitment(f.decision.Commitment)
	}
	if f.action != nil {
		chain.ActionCommitment = ledger.Commitment(f.action.Commitment)
	}
	result := phase5DVersion{local: local, chain: chain, submission: sub}
	f.versions = append(f.versions, result)
	return result
}

func phase5DSubmit(t *testing.T, ctx context.Context, service *Service, v phase5DVersion) {
	t.Helper()
	if err := service.Submit(ctx, v.submission.OperationID); err != nil {
		t.Fatal(err)
	}
}

func TestPhase5DFinalIntegratedLifecycle(t *testing.T) {
	ctx := context.Background()
	url := testDatabaseURL(t)
	store := phase5DOpen(t, ctx, url)
	migrations := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "migrations", "authority_ledger"))
	_ = migrations.DownAll(ctx)
	if err := migrations.Up(ctx); err != nil {
		t.Fatal(err)
	}
	var address authorityledger.Address
	organizations := []authorityledger.Organization{{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: address}, {ID: "PAYER-B", Type: "PAYER", DisplayName: "Payer B", Active: true, Signer: address}, {ID: "STAFFING-AGENCY-C", Type: "STAFFING_AGENCY", DisplayName: "Staffing Agency C", Active: true, Signer: address}}
	for _, org := range organizations {
		if err := store.CreateOrganization(ctx, org); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: address}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-7F31A", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEventSeries(ctx, authorityledger.EventSeries{ID: "SERIES-PHASE5D", SubjectID: "PRV-7F31A", AuthorityID: "HHS-OIG-DEMO"}); err != nil {
		t.Fatal(err)
	}
	effective := time.Date(2026, 4, 1, 0, 0, 0, 123, time.UTC)
	eventEvidence := canonical.AuthorityEventV1{EventID: "EVENT-PHASE5D", EventSeriesID: "SERIES-PHASE5D", AuthorityID: "HHS-OIG-DEMO", ProviderReference: "PRV-7F31A", EventType: canonical.EventTypeExclusion, AssertionKind: canonical.AssertionKindOriginal, AuthorityEffect: canonical.AuthorityEffectExclusionActive, EffectiveTime: effective, SourceDocumentHash: canonical.Hash([]byte("private synthetic authority source"))}
	eventHash, err := canonical.AuthorityEventCommitment(eventEvidence)
	if err != nil {
		t.Fatal(err)
	}
	assertion := authorityledger.AuthorityAssertion{ID: "EVENT-PHASE5D", SeriesID: "SERIES-PHASE5D", EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", EffectiveUnixNano: effective.UnixNano(), SourceDocument: []byte("private synthetic authority source"), Commitment: phase5DCanonicalCommitment(t, eventHash), CanonicalVersion: 1}
	assertionSub := phase5DSubmission("OP-EVENT-PHASE5D", OperationAuthorityAssertion, string(assertion.ID), "recordAssertion", "HHS-OIG-DEMO", assertion.Commitment)
	if err = store.CreateAssertionWithPending(ctx, assertion, assertionSub); err != nil {
		t.Fatal(err)
	}
	if len(assertion.Commitment) != 32 {
		t.Fatal("assertion commitment width")
	}
	authority := phase5DAdapter(t, "http://127.0.0.1:8545", testAuthorityKey)
	hospital := phase5DAdapter(t, "http://127.0.0.1:8545", testHospitalKey)
	payer := phase5DAdapter(t, "http://127.0.0.1:8545", phase5DPayerKey)
	staffing := phase5DAdapter(t, "http://127.0.0.1:8545", phase5DStaffingKey)
	signers := map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	service := New(store, signers)
	phase5DSubmit(t, ctx, service, phase5DVersion{submission: assertionSub})
	flows := []*phase5DFlow{{org: "HOSPITAL-A", short: "HOSP", responseID: "R-HOSP", receipt: time.Date(2026, 4, 2, 0, 0, 0, 1, time.UTC)}, {org: "PAYER-B", short: "PAYER", responseID: "R-PAYER", receipt: time.Date(2026, 4, 2, 0, 0, 0, 2, time.UTC)}, {org: "STAFFING-AGENCY-C", short: "STAFF", responseID: "R-STAFF", receipt: time.Date(2026, 4, 2, 0, 0, 0, 3, time.UTC)}}
	for _, flow := range flows {
		if err = store.CreateResponseStream(ctx, authorityledger.ResponseStream{ID: authorityledger.ResponseID(flow.responseID), EventID: assertion.ID, OrganizationID: flow.org}); err != nil {
			t.Fatal(err)
		}
	}
	h1 := flows[0].append(t, ctx, store, assertion.ID, 1)
	phase5DSubmit(t, ctx, service, h1)
	h2 := flows[0].append(t, ctx, store, assertion.ID, 2)
	phase5DSubmit(t, ctx, service, h2)
	h3 := flows[0].append(t, ctx, store, assertion.ID, 3)
	h3tx, err := hospital.BroadcastResponseVersion(ctx, h3.chain)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.MarkSubmitted(ctx, h3.submission.OperationID, h3tx[:], time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = hospital.WaitReceipt(ctx, h3tx); err != nil {
		t.Fatal(err)
	}
	p1 := flows[1].append(t, ctx, store, assertion.ID, 1)
	p1tx, err := payer.BroadcastResponseVersion(ctx, p1.chain)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = payer.WaitReceipt(ctx, p1tx); err != nil {
		t.Fatal(err)
	}
	s1 := flows[2].append(t, ctx, store, assertion.ID, 1)
	phase5DSubmit(t, ctx, service, s1)
	s2 := flows[2].append(t, ctx, store, assertion.ID, 2)
	s2tx, err := staffing.BroadcastResponseVersion(ctx, s2.chain)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.MarkSubmitted(ctx, s2.submission.OperationID, s2tx[:], time.Now()); err != nil {
		t.Fatal(err)
	}
	authority.Close()
	hospital.Close()
	payer.Close()
	staffing.Close()
	store.Close()
	store = phase5DOpen(t, ctx, url)
	authority = phase5DAdapter(t, "http://127.0.0.1:8545", testAuthorityKey)
	hospital = phase5DAdapter(t, "http://127.0.0.1:8545", testHospitalKey)
	payer = phase5DAdapter(t, "http://127.0.0.1:8545", phase5DPayerKey)
	staffing = phase5DAdapter(t, "http://127.0.0.1:8545", phase5DStaffingKey)
	staffingAfterRestart := &observedLedger{ReconciliationInterface: staffing, pendingOnce: true}
	reconciliationLedgers := map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffingAfterRestart}
	reconciler := NewReconciler(store, reconciliationLedgers)
	results := reconciler.ReconcileUnresolved(ctx, 10)
	if len(results) != 3 {
		t.Fatalf("restart recovered %d operations", len(results))
	}
	for _, result := range results {
		if result.OperationID == s2.submission.OperationID {
			if !errors.Is(result.Err, ErrReceiptPending) || result.Outcome != OutcomeStillPending {
				t.Fatalf("crash C first pass: %+v", result)
			}
		} else if result.Err != nil || result.Outcome != OutcomeRecovered {
			t.Fatalf("recovery %+v", result)
		}
	}
	for attempt := 0; attempt < 200; attempt++ {
		out, reconcileErr := reconciler.ReconcileOne(ctx, s2.submission.OperationID)
		if reconcileErr == nil && out == OutcomeRecovered {
			break
		}
		if !errors.Is(reconcileErr, ErrReceiptPending) {
			t.Fatalf("crash C retry: %s %v", out, reconcileErr)
		}
		if attempt == 199 {
			t.Fatal("crash C receipt remained pending")
		}
		time.Sleep(100 * time.Millisecond)
	}
	service = New(store, map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing})
	h4 := flows[0].append(t, ctx, store, assertion.ID, 4)
	phase5DSubmit(t, ctx, service, h4)
	for stage := 2; stage <= 4; stage++ {
		v := flows[1].append(t, ctx, store, assertion.ID, stage)
		phase5DSubmit(t, ctx, service, v)
	}
	for stage := 3; stage <= 4; stage++ {
		v := flows[2].append(t, ctx, store, assertion.ID, stage)
		phase5DSubmit(t, ctx, service, v)
	}
	for _, test := range []struct {
		name  string
		wrong *besu.Adapter
		value ledger.ResponseVersion
	}{{"hospital-payer", hospital, flows[1].versions[3].chain}, {"payer-staffing", payer, flows[2].versions[3].chain}, {"staffing-hospital", staffing, flows[0].versions[3].chain}} {
		if _, e := test.wrong.BroadcastResponseVersion(ctx, test.value); !errors.Is(e, ledger.ErrContractRevert) {
			t.Fatalf("%s authorization: %v", test.name, e)
		}
	}
	allSubs := []authorityledger.Submission{assertionSub}
	allExpected := make(map[authorityledger.OperationID]expectedRecord)
	assertionChain, err := assertionValue(assertion, "HHS-OIG-DEMO")
	if err != nil {
		t.Fatal(err)
	}
	allExpected[assertionSub.OperationID] = expectedRecord{operationType: OperationAuthorityAssertion, assertion: assertionChain}
	for _, flow := range flows {
		history, historyErr := store.ResponseHistory(ctx, authorityledger.ResponseID(flow.responseID))
		if historyErr != nil || len(history) != 4 {
			t.Fatalf("%s history=%d %v", flow.org, len(history), historyErr)
		}
		for i, v := range history {
			if i == 0 && v.PreviousID != nil {
				t.Fatal("non-root first response")
			}
			if i > 0 && (v.PreviousID == nil || *v.PreviousID != history[i-1].ID) {
				t.Fatalf("%s broken predecessor", flow.org)
			}
		}
		for _, v := range flow.versions {
			allSubs = append(allSubs, v.submission)
			allExpected[v.submission.OperationID] = expectedRecord{operationType: OperationResponseVersion, response: v.chain}
		}
	}
	for _, sub := range allSubs {
		stored, e := store.GetSubmission(ctx, sub.OperationID)
		if e != nil || stored.State != "CONFIRMED" || len(stored.TransactionHash) != 32 || stored.BlockNumber == nil || len(stored.BlockHash) != 32 {
			t.Fatalf("submission %s: %+v %v", sub.OperationID, stored, e)
		}
		lookup, e := lookupExpected(ctx, reconciliationLedgers[stored.SignerContext], allExpected[sub.OperationID])
		if e != nil || lookup.status != ledger.ChainRecordPresent || !lookup.matches || string(stored.TransactionHash) != string(lookup.receipt.Hash[:]) || *stored.BlockNumber != int64(lookup.receipt.BlockNumber) || string(stored.BlockHash) != string(lookup.receipt.BlockHash[:]) {
			t.Fatalf("chain consistency %s: %+v %v", sub.OperationID, lookup, e)
		}
	}
	for _, node := range []string{"http://127.0.0.1:8545", "http://127.0.0.1:8546", "http://127.0.0.1:8547"} {
		reader := phase5DAdapter(t, node, testAuthorityKey)
		onChain, e := reader.GetAuthorityAssertion(ctx, assertionChain.EventID)
		if e != nil || !equalAssertion(onChain, assertionChain) {
			t.Fatalf("%s assertion mismatch: %+v %v", node, onChain, e)
		}
		head, e := reader.GetCurrentAuthorityHead(ctx, assertionChain.EventSeriesID)
		if e != nil || head != assertionChain.EventID {
			t.Fatalf("%s head %s %v", node, head.String(), e)
		}
		for _, flow := range flows {
			for _, v := range flow.versions {
				onChainResponse, e := reader.GetResponseVersion(ctx, v.chain.ResponseVersionID)
				if e != nil || !equalResponse(onChainResponse, v.chain) {
					t.Fatalf("%s %s mismatch: %+v %v", node, v.local.ID, onChainResponse, e)
				}
			}
			latest, e := reader.GetLatestResponse(ctx, assertionChain.EventID, ledger.MustID(string(flow.org)))
			if e != nil || latest.ResponseVersionID != flow.versions[3].chain.ResponseVersionID {
				t.Fatalf("%s %s latest: %+v %v", node, flow.org, latest, e)
			}
		}
		reader.Close()
	}
	snapshots := make(map[authorityledger.OperationID]authorityledger.Submission)
	for _, sub := range allSubs {
		snapshots[sub.OperationID], _ = store.GetSubmission(ctx, sub.OperationID)
	}
	authority.Close()
	hospital.Close()
	payer.Close()
	staffing.Close()
	store.Close()
	store = phase5DOpen(t, ctx, url)
	authority = phase5DAdapter(t, "http://127.0.0.1:8545", testAuthorityKey)
	hospital = phase5DAdapter(t, "http://127.0.0.1:8545", testHospitalKey)
	payer = phase5DAdapter(t, "http://127.0.0.1:8545", phase5DPayerKey)
	staffing = phase5DAdapter(t, "http://127.0.0.1:8545", phase5DStaffingKey)
	reconciler = NewReconciler(store, map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing})
	if unresolved, e := store.Unresolved(ctx); e != nil || len(unresolved) != 0 {
		t.Fatalf("second restart unresolved=%d %v", len(unresolved), e)
	}
	for id, before := range snapshots {
		out, e := reconciler.ReconcileOne(ctx, id)
		after, _ := store.GetSubmission(ctx, id)
		if e != nil || out != OutcomeAlreadyConfirmed || after.AttemptCount != before.AttemptCount || string(after.TransactionHash) != string(before.TransactionHash) || string(after.BlockHash) != string(before.BlockHash) || *after.BlockNumber != *before.BlockNumber {
			t.Fatalf("idempotency %s: %s %+v %+v %v", id, out, before, after, e)
		}
	}
	if count, e := store.Count(ctx, "ledger_submissions"); e != nil || count != 13 {
		t.Fatalf("submission count=%d %v", count, e)
	}
	authority.Close()
	hospital.Close()
	payer.Close()
	staffing.Close()
	finalMigrations := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "migrations", "authority_ledger"))
	if err = finalMigrations.DownAll(ctx); err != nil {
		t.Fatal(err)
	}
	store.Close()
}
