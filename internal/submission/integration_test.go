//go:build integration

package submission

import (
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

const (
	testAuthorityKey = "00000000000000000000000000000000000000000000000000000000000a11ce"
	testHospitalKey  = "0000000000000000000000000000000000000000000000000000000000000b0b"
)

var testContracts = besu.ContractAddresses{
	OrganizationRegistry:   common.HexToAddress("0x1148e16d870cc191b97774d1b55f65d42ecf6cd7"),
	AuthorityRegistry:      common.HexToAddress("0xf5495ad34a7f25b26bb8b8ef512c44c7cb5cc005"),
	AuthorityEventRegistry: common.HexToAddress("0x3372e3af8edce113af8926a924f823edf2a301c2"),
	ResponseLedger:         common.HexToAddress("0xaebe3a5fc1b9049970d0a53967d07c98211bcde7"),
}

func adapter(t *testing.T, rpc, key string) *besu.Adapter {
	t.Helper()
	a, err := besu.New(context.Background(), besu.Config{RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: testContracts, PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(a.Close)
	return a
}
func commitment(v canonical.Commitment) (out authorityledger.Commitment) { copy(out[:], v[:]); return }

func TestPostgresBesuNormalSubmissionAndSecondNodeRead(t *testing.T) {
	databaseURL := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	store, err := authorityledger.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	migrations := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "migrations", "authority_ledger"))
	_ = migrations.DownAll(ctx)
	if err = migrations.Up(ctx); err != nil {
		t.Fatal(err)
	}
	defer migrations.DownAll(ctx)
	var signer authorityledger.Address
	if err = store.CreateOrganization(ctx, authorityledger.Organization{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: signer}); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: signer}); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-PHASE5B", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateEventSeries(ctx, authorityledger.EventSeries{ID: "SERIES-PHASE5B", SubjectID: "PRV-PHASE5B", AuthorityID: "HHS-OIG-DEMO"}); err != nil {
		t.Fatal(err)
	}

	effective := time.Date(2026, 2, 1, 0, 0, 0, 123, time.UTC)
	evidence := canonical.AuthorityEventV1{EventID: "EVENT-PHASE5B", EventSeriesID: "SERIES-PHASE5B", AuthorityID: "HHS-OIG-DEMO", ProviderReference: "PRV-PHASE5B", EventType: canonical.EventTypeExclusion, AssertionKind: canonical.AssertionKindOriginal, AuthorityEffect: canonical.AuthorityEffectExclusionActive, EffectiveTime: effective, SourceDocumentHash: canonical.Hash([]byte("private synthetic source"))}
	eventHash, err := canonical.AuthorityEventCommitment(evidence)
	if err != nil {
		t.Fatal(err)
	}
	assertion := authorityledger.AuthorityAssertion{ID: "EVENT-PHASE5B", SeriesID: "SERIES-PHASE5B", EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", EffectiveUnixNano: effective.UnixNano(), SourceDocument: []byte("private synthetic source"), Commitment: commitment(eventHash), CanonicalVersion: 1}
	assertionSubmission := authorityledger.Submission{OperationID: "OP-EVENT-PHASE5B", OperationType: OperationAuthorityAssertion, DomainVersionID: string(assertion.ID), ContractAction: "recordAssertion", SignerContext: "HHS-OIG-DEMO", IntendedCommitment: assertion.Commitment, State: "LOCAL_PENDING"}
	if err = store.CreateAssertionWithPending(ctx, assertion, assertionSubmission); err != nil {
		t.Fatal(err)
	}
	authority := adapter(t, "http://127.0.0.1:8545", testAuthorityKey)
	hospital := adapter(t, "http://127.0.0.1:8545", testHospitalKey)
	service := New(store, map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital})
	if err = service.Submit(ctx, assertionSubmission.OperationID); err != nil {
		t.Fatal(err)
	}
	confirmed, err := store.GetSubmission(ctx, assertionSubmission.OperationID)
	if err != nil || confirmed.State != "CONFIRMED" || len(confirmed.TransactionHash) != 32 || confirmed.BlockNumber == nil || len(confirmed.BlockHash) != 32 {
		t.Fatalf("assertion submission: %+v %v", confirmed, err)
	}

	if err = store.CreateResponseStream(ctx, authorityledger.ResponseStream{ID: "RESPONSE-PHASE5B", EventID: assertion.ID, OrganizationID: "HOSPITAL-A"}); err != nil {
		t.Fatal(err)
	}
	receivedAt := time.Date(2026, 2, 2, 0, 0, 0, 456, time.UTC)
	responseEvidence := canonical.ResponseV1{ResponseID: "RESPONSE-PHASE5B", ResponseVersionID: "RESPONSE-PHASE5B-V1", EventID: string(assertion.ID), OrganizationID: "HOSPITAL-A", ResponseState: canonical.ResponseStateReceived, ReceiptTimestamp: receivedAt}
	responseHash, err := canonical.ResponseCommitment(responseEvidence)
	if err != nil {
		t.Fatal(err)
	}
	response := authorityledger.ResponseVersion{ID: "RESPONSE-PHASE5B-V1", ResponseID: "RESPONSE-PHASE5B", EventID: assertion.ID, OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: receivedAt.UnixNano(), Commitment: commitment(responseHash), CanonicalVersion: 1}
	responseSubmission := authorityledger.Submission{OperationID: "OP-RESPONSE-PHASE5B", OperationType: OperationResponseVersion, DomainVersionID: string(response.ID), ContractAction: "recordResponseVersion", SignerContext: "HOSPITAL-A", IntendedCommitment: response.Commitment, State: "LOCAL_PENDING"}
	if err = store.CreateResponseWithPending(ctx, response, responseSubmission); err != nil {
		t.Fatal(err)
	}
	if err = service.Submit(ctx, responseSubmission.OperationID); err != nil {
		t.Fatal(err)
	}
	confirmed, err = store.GetSubmission(ctx, responseSubmission.OperationID)
	if err != nil || confirmed.State != "CONFIRMED" || confirmed.BlockNumber == nil {
		t.Fatalf("response submission: %+v %v", confirmed, err)
	}

	peer := adapter(t, "http://127.0.0.1:8546", testAuthorityKey)
	onChainAssertion, err := peer.GetAuthorityAssertion(ctx, ledger.MustID("EVENT-PHASE5B"))
	if err != nil || onChainAssertion.EventCommitment != ledger.Commitment(assertion.Commitment) {
		t.Fatalf("peer assertion: %+v %v", onChainAssertion, err)
	}
	onChainResponse, err := peer.GetLatestResponse(ctx, ledger.MustID("EVENT-PHASE5B"), ledger.MustID("HOSPITAL-A"))
	if err != nil || onChainResponse.ResponseVersionID != ledger.MustID("RESPONSE-PHASE5B-V1") || onChainResponse.State != ledger.ResponseReceived || onChainResponse.ReceiptTimestamp != response.ReceiptUnixNano {
		t.Fatalf("peer response: %+v %v", onChainResponse, err)
	}
	storedResponse, err := store.GetResponse(ctx, response.ID)
	if err != nil || storedResponse.Commitment != response.Commitment {
		t.Fatalf("stored response commitment: %+v %v", storedResponse, err)
	}
}
