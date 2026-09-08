//go:build integration

package besu

import (
	"context"
	"errors"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

const (
	deployerKey  = "000000000000000000000000000000000000000000000000000000000000b007"
	authorityKey = "00000000000000000000000000000000000000000000000000000000000a11ce"
	hospitalKey  = "0000000000000000000000000000000000000000000000000000000000000b0b"
	payerKey     = "000000000000000000000000000000000000000000000000000000000000ca11"
)

var localContracts = ContractAddresses{
	OrganizationRegistry:   common.HexToAddress("0x1148e16d870cc191b97774d1b55f65d42ecf6cd7"),
	AuthorityRegistry:      common.HexToAddress("0xf5495ad34a7f25b26bb8b8ef512c44c7cb5cc005"),
	AuthorityEventRegistry: common.HexToAddress("0x3372e3af8edce113af8926a924f823edf2a301c2"),
	ResponseLedger:         common.HexToAddress("0xaebe3a5fc1b9049970d0a53967d07c98211bcde7"),
}

func localConfig(rpc, key string) Config {
	return Config{RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: localContracts, PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond}
}
func testAddress(t *testing.T, key string) (out ledger.Address) {
	parsed, err := crypto.HexToECDSA(key)
	if err != nil {
		t.Fatal(err)
	}
	copy(out[:], crypto.PubkeyToAddress(parsed.PublicKey).Bytes())
	return
}

func TestBesuAuthorityCommitmentResponseAndMultiNodeRead(t *testing.T) {
	ctx := context.Background()
	admin, err := New(ctx, localConfig("http://127.0.0.1:8545", deployerKey))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	authority, err := New(ctx, localConfig("http://127.0.0.1:8545", authorityKey))
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	hospital, err := New(ctx, localConfig("http://127.0.0.1:8545", hospitalKey))
	if err != nil {
		t.Fatal(err)
	}
	defer hospital.Close()

	orgID := ledger.MustID("HOSPITAL-A")
	authID := ledger.MustID("HHS-OIG-DEMO")
	if got, err := admin.GetOrganization(ctx, orgID); err != nil || !got.Active {
		t.Fatalf("organization: %+v %v", got, err)
	}
	if got, err := admin.GetAuthority(ctx, authID); err != nil || !got.Active {
		t.Fatalf("authority: %+v %v", got, err)
	}

	eventID := ledger.MustID("EVENT-GO-PHASE4")
	seriesID := ledger.MustID("SERIES-GO-PHASE4")
	evidence := canonical.AuthorityEventV1{EventID: eventID.String(), EventSeriesID: seriesID.String(), AuthorityID: authID.String(), ProviderReference: "PRV-7F31A", EventType: canonical.EventTypeExclusion, AssertionKind: canonical.AssertionKindOriginal, AuthorityEffect: canonical.AuthorityEffectExclusionActive, EffectiveTime: time.Date(2026, 1, 1, 0, 0, 0, 123, time.UTC), SourceDocumentHash: canonical.Hash([]byte("synthetic authority source"))}
	commitment, err := canonical.AuthorityEventCommitment(evidence)
	if err != nil {
		t.Fatal(err)
	}
	var ledgerCommitment ledger.Commitment
	copy(ledgerCommitment[:], commitment[:])
	tx, err := authority.RecordAuthorityAssertion(ctx, ledger.AuthorityAssertion{EventID: eventID, EventSeriesID: seriesID, AuthorityID: authID, EventCommitment: ledgerCommitment, EventType: ledger.EventTypeExclusion, AssertionKind: ledger.AssertionOriginal, AuthorityEffect: ledger.EffectExclusionActive, EffectiveTime: evidence.EffectiveTime.UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if tx.BlockNumber == 0 || tx.Hash == (ledger.TransactionHash{}) {
		t.Fatal("missing confirmed transaction metadata")
	}
	stored, err := authority.GetAuthorityAssertion(ctx, eventID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.EventCommitment != ledgerCommitment {
		t.Fatal("commitment mismatch")
	}
	head, err := authority.GetCurrentAuthorityHead(ctx, seriesID)
	if err != nil || head != eventID {
		t.Fatalf("head: %s %v", head.String(), err)
	}
	receipt, err := authority.TransactionReceipt(ctx, tx.Hash)
	if err != nil || receipt.Status != ledger.ReceiptSuccessful || receipt.BlockNumber != tx.BlockNumber {
		t.Fatalf("receipt: %+v %v", receipt, err)
	}

	responseID := ledger.MustID("RESPONSE-GO-PHASE4")
	versionID := ledger.MustID("RESPONSE-GO-PHASE4-V1")
	rtx, err := hospital.AppendResponseVersion(ctx, ledger.ResponseVersion{ResponseID: responseID, ResponseVersionID: versionID, EventID: eventID, OrganizationID: orgID, State: ledger.ResponseReceived, ReceiptTimestamp: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC).UnixNano()})
	if err != nil {
		t.Fatal(err)
	}
	if rtx.BlockNumber == 0 {
		t.Fatal("response receipt has no block")
	}
	latest, err := hospital.GetLatestResponse(ctx, eventID, orgID)
	if err != nil || latest.ResponseVersionID != versionID {
		t.Fatalf("latest: %+v %v", latest, err)
	}

	peer, err := New(ctx, localConfig("http://127.0.0.1:8546", authorityKey))
	if err != nil {
		t.Fatal(err)
	}
	defer peer.Close()
	peerStored, err := peer.GetAuthorityAssertion(ctx, eventID)
	if err != nil || peerStored.EventCommitment != ledgerCommitment {
		t.Fatalf("peer read: %+v %v", peerStored, err)
	}
	peerLatest, err := peer.GetLatestResponse(ctx, eventID, orgID)
	if err != nil || peerLatest.ResponseVersionID != versionID {
		t.Fatalf("peer latest: %+v %v", peerLatest, err)
	}
}

func TestNegativeBesuCases(t *testing.T) {
	ctx := context.Background()
	cfg := localConfig("http://127.0.0.1:8545", authorityKey)
	cfg.ExpectedChainID = big.NewInt(1)
	if _, err := New(ctx, cfg); !errors.Is(err, ledger.ErrChainIDMismatch) {
		t.Fatalf("chain mismatch: %v", err)
	}

	unauthorized, err := New(ctx, localConfig("http://127.0.0.1:8545", payerKey))
	if err != nil {
		t.Fatal(err)
	}
	defer unauthorized.Close()
	_, err = unauthorized.RecordAuthorityAssertion(ctx, ledger.AuthorityAssertion{EventID: ledger.MustID("EVENT-UNAUTHORIZED"), EventSeriesID: ledger.MustID("SERIES-UNAUTHORIZED"), AuthorityID: ledger.MustID("HHS-OIG-DEMO"), EventCommitment: ledger.Commitment{1}, EventType: 1, AssertionKind: 1, AuthorityEffect: 1, EffectiveTime: 1})
	if !errors.Is(err, ledger.ErrContractRevert) {
		t.Fatalf("unauthorized: %v", err)
	}

	hospital, err := New(ctx, localConfig("http://127.0.0.1:8545", hospitalKey))
	if err != nil {
		t.Fatal(err)
	}
	defer hospital.Close()
	_, err = hospital.AppendResponseVersion(ctx, ledger.ResponseVersion{ResponseID: ledger.MustID("BAD-RESPONSE"), ResponseVersionID: ledger.MustID("BAD-RESPONSE-V1"), EventID: ledger.MustID("EVENT-GO-PHASE4"), OrganizationID: ledger.MustID("HOSPITAL-A"), State: ledger.ResponseDecided, ReceiptTimestamp: 1, DecisionCommitment: ledger.Commitment{1}})
	if !errors.Is(err, ledger.ErrContractRevert) {
		t.Fatalf("invalid transition: %v", err)
	}

	hospital.timeout = time.Nanosecond
	if _, err = hospital.wait(ctx, common.HexToHash("0x1234")); !errors.Is(err, ledger.ErrReceiptTimeout) {
		t.Fatalf("timeout: %v", err)
	}
	_ = testAddress(t, hospitalKey)
}
