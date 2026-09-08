//go:build integration

package submission

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	value := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if value == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	return value
}
func openMigratedStore(t *testing.T, ctx context.Context, url string) *authorityledger.Store {
	t.Helper()
	store, err := authorityledger.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	m := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "migrations", "authority_ledger"))
	_ = m.DownAll(ctx)
	if err = m.Up(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.DownAll(ctx); store.Close() })
	return store
}
func seedRecoveryStore(t *testing.T, ctx context.Context, store *authorityledger.Store) {
	t.Helper()
	var signer authorityledger.Address
	if err := store.CreateOrganization(ctx, authorityledger.Organization{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: signer}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: signer}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-PHASE5C", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := store.CreateEventSeries(ctx, authorityledger.EventSeries{ID: "SERIES-PHASE5C", SubjectID: "PRV-PHASE5C", AuthorityID: "HHS-OIG-DEMO"}); err != nil {
		t.Fatal(err)
	}
}

type observedLedger struct {
	ledger.ReconciliationInterface
	pendingOnce bool
	broadcasts  int
}

func (o *observedLedger) BroadcastAuthorityAssertion(c context.Context, v ledger.AuthorityAssertion) (ledger.TransactionHash, error) {
	o.broadcasts++
	return o.ReconciliationInterface.BroadcastAuthorityAssertion(c, v)
}
func (o *observedLedger) BroadcastResponseVersion(c context.Context, v ledger.ResponseVersion) (ledger.TransactionHash, error) {
	o.broadcasts++
	return o.ReconciliationInterface.BroadcastResponseVersion(c, v)
}
func (o *observedLedger) ObserveReceipt(c context.Context, h ledger.TransactionHash) (ledger.Receipt, error) {
	if o.pendingOnce {
		o.pendingOnce = false
		return ledger.Receipt{Transaction: ledger.Transaction{Hash: h}, Status: ledger.ReceiptUnknown}, nil
	}
	return o.ReconciliationInterface.ObserveReceipt(c, h)
}

func TestPhase5CCrashRecoveryWithPostgresAndBesu(t *testing.T) {
	databaseURL := testDatabaseURL(t)
	ctx := context.Background()
	store := openMigratedStore(t, ctx, databaseURL)
	seedRecoveryStore(t, ctx, store)
	authority := adapter(t, "http://127.0.0.1:8545", testAuthorityKey)
	hospital := adapter(t, "http://127.0.0.1:8545", testHospitalKey)
	authorityObserved := &observedLedger{ReconciliationInterface: authority}
	hospitalObserved := &observedLedger{ReconciliationInterface: hospital}

	effective := time.Date(2026, 3, 1, 0, 0, 0, 123, time.UTC)
	evidence := canonical.AuthorityEventV1{EventID: "EVENT-PHASE5C", EventSeriesID: "SERIES-PHASE5C", AuthorityID: "HHS-OIG-DEMO", ProviderReference: "PRV-PHASE5C", EventType: canonical.EventTypeExclusion, AssertionKind: canonical.AssertionKindOriginal, AuthorityEffect: canonical.AuthorityEffectExclusionActive, EffectiveTime: effective, SourceDocumentHash: canonical.Hash([]byte("private recovery source"))}
	eventHash, err := canonical.AuthorityEventCommitment(evidence)
	if err != nil {
		t.Fatal(err)
	}
	assertion := authorityledger.AuthorityAssertion{ID: "EVENT-PHASE5C", SeriesID: "SERIES-PHASE5C", EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", EffectiveUnixNano: effective.UnixNano(), SourceDocument: []byte("private recovery source"), Commitment: commitment(eventHash), CanonicalVersion: 1}
	sub := authorityledger.Submission{OperationID: "OP-EVENT-PHASE5C", OperationType: OperationAuthorityAssertion, DomainVersionID: string(assertion.ID), ContractAction: "recordAssertion", SignerContext: "HHS-OIG-DEMO", IntendedCommitment: assertion.Commitment, State: "LOCAL_PENDING"}
	if err = store.CreateAssertionWithPending(ctx, assertion, sub); err != nil {
		t.Fatal(err)
	}
	chainValue, err := assertionValue(assertion, "HHS-OIG-DEMO")
	if err != nil {
		t.Fatal(err)
	}
	broadcastHash, err := authority.BroadcastAuthorityAssertion(ctx, chainValue)
	if err != nil {
		t.Fatal(err)
	}
	mined, err := authority.WaitReceipt(ctx, broadcastHash)
	if err != nil {
		t.Fatal(err)
	}
	reconciler := NewReconciler(store, map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authorityObserved, "HOSPITAL-A": hospitalObserved})
	out, err := reconciler.ReconcileOne(ctx, sub.OperationID)
	if err != nil || out != OutcomeRecovered {
		t.Fatalf("crash B: %s %v", out, err)
	}
	recovered, err := store.GetSubmission(ctx, sub.OperationID)
	if err != nil || recovered.State != "CONFIRMED" || string(recovered.TransactionHash) != string(broadcastHash[:]) || recovered.BlockNumber == nil || *recovered.BlockNumber != int64(mined.BlockNumber) || string(recovered.BlockHash) != string(mined.BlockHash[:]) || recovered.AttemptCount != 0 || authorityObserved.broadcasts != 0 {
		t.Fatalf("crash B recovery: %+v broadcasts=%d err=%v", recovered, authorityObserved.broadcasts, err)
	}

	if err = store.CreateResponseStream(ctx, authorityledger.ResponseStream{ID: "RESPONSE-PHASE5C", EventID: assertion.ID, OrganizationID: "HOSPITAL-A"}); err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 3, 2, 0, 0, 0, 456, time.UTC)
	responseEvidence := canonical.ResponseV1{ResponseID: "RESPONSE-PHASE5C", ResponseVersionID: "RESPONSE-PHASE5C-V1", EventID: string(assertion.ID), OrganizationID: "HOSPITAL-A", ResponseState: canonical.ResponseStateReceived, ReceiptTimestamp: receivedAt}
	responseHash, err := canonical.ResponseCommitment(responseEvidence)
	if err != nil {
		t.Fatal(err)
	}
	response := authorityledger.ResponseVersion{ID: "RESPONSE-PHASE5C-V1", ResponseID: "RESPONSE-PHASE5C", EventID: assertion.ID, OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: receivedAt.UnixNano(), Commitment: commitment(responseHash), CanonicalVersion: 1}
	responseSub := authorityledger.Submission{OperationID: "OP-RESPONSE-PHASE5C-V1", OperationType: OperationResponseVersion, DomainVersionID: string(response.ID), ContractAction: "recordResponseVersion", SignerContext: "HOSPITAL-A", IntendedCommitment: response.Commitment, State: "LOCAL_PENDING"}
	if err = store.CreateResponseWithPending(ctx, response, responseSub); err != nil {
		t.Fatal(err)
	}
	normal := New(store, map[string]ledger.SubmissionInterface{"HOSPITAL-A": hospital})
	responseValue, err := normal.responseValue(ctx, response)
	if err != nil {
		t.Fatal(err)
	}
	responseTx, err := hospital.BroadcastResponseVersion(ctx, responseValue)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.MarkSubmitted(ctx, responseSub.OperationID, responseTx[:], time.Now()); err != nil {
		t.Fatal(err)
	}
	hospitalObserved.pendingOnce = true
	out, err = reconciler.ReconcileOne(ctx, responseSub.OperationID)
	if out != OutcomeStillPending || !errorsIs(err, ErrReceiptPending) {
		t.Fatalf("crash C pending: %s %v", out, err)
	}
	pending, _ := store.GetSubmission(ctx, responseSub.OperationID)
	if pending.State != "SUBMITTED" || pending.AttemptCount != 1 || hospitalObserved.broadcasts != 0 {
		t.Fatalf("crash C state: %+v broadcasts=%d", pending, hospitalObserved.broadcasts)
	}
	if _, err = hospital.WaitReceipt(ctx, responseTx); err != nil {
		t.Fatal(err)
	}
	out, err = reconciler.ReconcileOne(ctx, responseSub.OperationID)
	if err != nil || out != OutcomeRecovered {
		t.Fatalf("crash C confirmed: %s %v", out, err)
	}

	policyHash := commitment(canonical.Hash([]byte("policy commitment")))
	policy := authorityledger.Policy{ID: "POLICY-PHASE5C", VersionID: "POLICY-PHASE5C-V1", OrganizationID: "HOSPITAL-A", EffectiveFromUnixNano: receivedAt.UnixNano(), Text: "private policy", Commitment: policyHash, CanonicalVersion: 1}
	if err = store.CreatePolicy(ctx, policy); err != nil {
		t.Fatal(err)
	}
	previous := response.ID
	policyID := policy.VersionID
	review := authorityledger.ResponseVersion{ID: "RESPONSE-PHASE5C-V2", ResponseID: response.ResponseID, EventID: assertion.ID, OrganizationID: "HOSPITAL-A", PreviousID: &previous, State: "UNDER_REVIEW", ReceiptUnixNano: receivedAt.UnixNano(), PolicyVersionID: &policyID, Commitment: commitment(canonical.Hash([]byte("response v2 commitment"))), CanonicalVersion: 1}
	reviewSub := authorityledger.Submission{OperationID: "OP-RESPONSE-PHASE5C-V2", OperationType: OperationResponseVersion, DomainVersionID: string(review.ID), ContractAction: "recordResponseVersion", SignerContext: "HOSPITAL-A", IntendedCommitment: review.Commitment, State: "LOCAL_PENDING"}
	if err = store.CreateResponseWithPending(ctx, review, reviewSub); err != nil {
		t.Fatal(err)
	}
	reviewValue, err := normal.responseValue(ctx, review)
	if err != nil {
		t.Fatal(err)
	}
	reviewTx, err := hospital.BroadcastResponseVersion(ctx, reviewValue)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.MarkSubmitted(ctx, reviewSub.OperationID, reviewTx[:], time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = hospital.WaitReceipt(ctx, reviewTx); err != nil {
		t.Fatal(err)
	}
	out, err = reconciler.ReconcileOne(ctx, reviewSub.OperationID)
	if err != nil || out != OutcomeRecovered {
		t.Fatalf("crash D: %s %v", out, err)
	}

	peer := adapter(t, "http://127.0.0.1:8546", testAuthorityKey)
	peerAssertion, err := peer.LookupAuthorityAssertion(ctx, ledger.MustID("EVENT-PHASE5C"))
	if err != nil || peerAssertion.Status != ledger.ChainRecordPresent || peerAssertion.Assertion.EventCommitment != ledger.Commitment(assertion.Commitment) {
		t.Fatalf("peer assertion: %+v %v", peerAssertion, err)
	}
	peerResponse, err := peer.LookupResponseVersion(ctx, ledger.MustID("RESPONSE-PHASE5C-V2"))
	if err != nil || peerResponse.Status != ledger.ChainRecordPresent || peerResponse.Response.State != ledger.ResponseUnderReview || peerResponse.Response.PolicyVersionHash != ledger.Commitment(policy.Commitment) {
		t.Fatalf("peer response: %+v %v", peerResponse, err)
	}
}

func errorsIs(err, target error) bool { return err != nil && errors.Is(err, target) }
