package integrityverification

import (
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

func TestAggregateStatusPrecedence(t *testing.T) {
	if aggregate(StatusVerified, StatusIndeterminate) != StatusIndeterminate {
		t.Fatal("indeterminate did not propagate")
	}
	if aggregate(StatusIndeterminate, StatusFailed) != StatusFailed {
		t.Fatal("failure did not take precedence")
	}
}

func TestTransactionEvidenceMismatchMatrix(t *testing.T) {
	block := int64(7)
	txHash := []byte{1}
	txHash = append(txHash, make([]byte, 31)...)
	blockHash := []byte{2}
	blockHash = append(blockHash, make([]byte, 31)...)
	sub := authorityledger.Submission{TransactionHash: txHash, BlockNumber: &block, BlockHash: blockHash}
	good := ledger.Receipt{Transaction: ledger.Transaction{Hash: ledger.TransactionHash{1}, BlockNumber: 7, BlockHash: ledger.TransactionHash{2}}, Status: ledger.ReceiptSuccessful}
	if check, unavailable := transactionEvidence("V1", sub, good); check.Status != StatusVerified || unavailable {
		t.Fatalf("good receipt: %+v", check)
	}
	tests := []struct {
		name string
		edit func(*ledger.Receipt)
		want VerificationStatus
	}{
		{"receipt pending", func(v *ledger.Receipt) { v.Status = ledger.ReceiptUnknown }, StatusIndeterminate},
		{"failed receipt", func(v *ledger.Receipt) { v.Status = ledger.ReceiptReverted }, StatusFailed},
		{"transaction hash", func(v *ledger.Receipt) { v.Hash = ledger.TransactionHash{9} }, StatusFailed},
		{"block number", func(v *ledger.Receipt) { v.BlockNumber = 8 }, StatusFailed},
		{"block hash", func(v *ledger.Receipt) { v.BlockHash = ledger.TransactionHash{9} }, StatusFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := good
			test.edit(&value)
			check, _ := transactionEvidence("V1", sub, value)
			if check.Status != test.want {
				t.Fatalf("got %+v", check)
			}
		})
	}
}

func TestResponseSnapshotMismatchMatrix(t *testing.T) {
	previous := authorityledger.ResponseVersionID("V0")
	local := authorityledger.ResponseVersion{ID: "V1", ResponseID: "R1", EventID: "E1", OrganizationID: "HOSPITAL-A", PreviousID: &previous, State: "DECIDED", ReceiptUnixNano: 44}
	policy, decision, action := canonical.Commitment{1}, canonical.Commitment{2}, canonical.Commitment{3}
	evidence := responseEvidence{policy: &policy, decision: &decision, action: &action}
	chain := ledger.ResponseVersion{ResponseID: ledger.MustID("R1"), ResponseVersionID: ledger.MustID("V1"), PreviousResponseVersionID: ledger.MustID("V0"), EventID: ledger.MustID("E1"), OrganizationID: ledger.MustID("HOSPITAL-A"), State: ledger.ResponseDecided, ReceiptTimestamp: 44, PolicyVersionHash: ledger.Commitment{1}, DecisionCommitment: ledger.Commitment{2}, ActionCommitment: ledger.Commitment{3}}
	assertFailed := func(name string, edit func(*ledger.ResponseVersion)) {
		t.Helper()
		t.Run(name, func(t *testing.T) {
			value := chain
			edit(&value)
			if statusOf(responseSnapshotChecks(local, value, evidence)) != StatusFailed {
				t.Fatal("mismatch was not detected")
			}
		})
	}
	assertFailed("response identity", func(v *ledger.ResponseVersion) { v.ResponseID = ledger.MustID("OTHER") })
	assertFailed("organization", func(v *ledger.ResponseVersion) { v.OrganizationID = ledger.MustID("PAYER-B") })
	assertFailed("event", func(v *ledger.ResponseVersion) { v.EventID = ledger.MustID("OTHER") })
	assertFailed("state", func(v *ledger.ResponseVersion) { v.State = ledger.ResponseReceived })
	assertFailed("predecessor", func(v *ledger.ResponseVersion) { v.PreviousResponseVersionID = ledger.MustID("OTHER") })
	assertFailed("policy", func(v *ledger.ResponseVersion) { v.PolicyVersionHash = ledger.Commitment{9} })
	assertFailed("decision", func(v *ledger.ResponseVersion) { v.DecisionCommitment = ledger.Commitment{9} })
	assertFailed("action", func(v *ledger.ResponseVersion) { v.ActionCommitment = ledger.Commitment{9} })
}

func TestLocalResponseLineageRejectsRewriteAndSkipping(t *testing.T) {
	previous := authorityledger.ResponseVersionID("V1")
	history := []authorityledger.ResponseVersion{
		{ID: "V1", ResponseID: "R1", EventID: "E1", OrganizationID: "HOSPITAL-A", State: "RECEIVED"},
		{ID: "V2", ResponseID: "R1", EventID: "E1", OrganizationID: "HOSPITAL-A", PreviousID: &previous, State: "DECIDED"},
	}
	if statusOf(localResponseLineage(history)) != StatusFailed {
		t.Fatal("skipped state was not detected")
	}
	history[1].State = "UNDER_REVIEW"
	history[1].OrganizationID = "PAYER-B"
	if statusOf(localResponseLineage(history)) != StatusFailed {
		t.Fatal("changed organization was not detected")
	}
}
