package blockchain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	trustcrypto "github.com/healthtrust-exchange/healthtrust-exchange/internal/cryptography"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

var testTime = time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("test", 90*60))

func hashPayload(value string) []byte { hash := sha256.Sum256([]byte(value)); return hash[:] }

func newIdentityChain(t *testing.T, role identity.Role, status identity.ActorStatus) (*Blockchain, ed25519.PrivateKey) {
	t.Helper()
	publicKey, privateKey, err := trustcrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	registry := identity.NewMemoryRegistry()
	if err := registry.RegisterOrganization(identity.Organization{ID: "hospital-a", Name: "Hospital A", Status: identity.OrganizationActive}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterActor(identity.Actor{ID: "actor-a", OrganizationID: "hospital-a", Role: role, PublicKey: publicKey, Status: status}); err != nil {
		t.Fatal(err)
	}
	return New(testTime.Add(-time.Hour), registry, authorization.HealthcarePolicy{}), privateKey
}

func signedTransaction(t *testing.T, privateKey ed25519.PrivateKey, actorID, organizationID string, txType TransactionType) Transaction {
	t.Helper()
	tx := Transaction{ID: "event-001", Type: txType, ActorID: actorID, OrganizationID: organizationID, ResourceID: "resource-fictional-001", PayloadHash: hashPayload("fictional off-chain data"), Timestamp: testTime}
	if err := tx.Sign(privateKey); err != nil {
		t.Fatal(err)
	}
	return tx
}

func TestTransactionHashAndSignature(t *testing.T) {
	_, privateKey, err := trustcrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	tx := signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)
	copyWithUTC := tx
	copyWithUTC.Timestamp = tx.Timestamp.UTC()
	if tx.Hash() != copyWithUTC.Hash() || tx.Hash() != tx.Hash() {
		t.Fatal("transaction hash is not deterministic")
	}
	if !tx.VerifySignature() {
		t.Fatal("valid signature rejected")
	}
	tx.ResourceID = "modified"
	if tx.VerifySignature() {
		t.Fatal("modified signed transaction accepted")
	}
}

func TestBlockHashDeterministic(t *testing.T) {
	_, privateKey, _ := trustcrypto.GenerateKeyPair()
	tx := signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)
	first := NewBlock(1, testTime, [32]byte{1}, []Transaction{tx})
	second := NewBlock(1, testTime.UTC(), [32]byte{1}, []Transaction{tx})
	if first.Hash != second.Hash {
		t.Fatal("identical blocks produced different hashes")
	}
	second.Timestamp = second.Timestamp.Add(time.Nanosecond)
	if first.Hash == second.CalculateHash() {
		t.Fatal("modified block produced same hash")
	}
}

func TestRolePermissions(t *testing.T) {
	tests := []struct {
		name    string
		role    identity.Role
		txType  TransactionType
		allowed bool
	}{
		{"doctor commits record", identity.RoleDoctor, RecordCommitted, true},
		{"patient grants consent", identity.RolePatient, ConsentGranted, true},
		{"doctor cannot grant consent", identity.RoleDoctor, ConsentGranted, false},
		{"patient cannot commit record", identity.RolePatient, RecordCommitted, false},
		{"staff creates referral", identity.RoleStaff, ReferralCreated, true},
		{"admin has no domain permission", identity.RoleHospitalAdmin, RecordCommitted, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, privateKey := newIdentityChain(t, tt.role, identity.ActorActive)
			_, err := chain.AddBlock(testTime, []Transaction{signedTransaction(t, privateKey, "actor-a", "hospital-a", tt.txType)})
			if tt.allowed && err != nil {
				t.Fatalf("valid transaction rejected: %v", err)
			}
			if !tt.allowed && !errors.Is(err, authorization.ErrUnauthorizedTransaction) {
				t.Fatalf("error = %v, want unauthorized", err)
			}
		})
	}
}

func TestIdentityValidationRejections(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *Blockchain, *Transaction)
		want   error
	}{
		{"unknown actor", func(_ *testing.T, _ *Blockchain, tx *Transaction) { tx.ActorID = "unknown" }, identity.ErrActorNotFound},
		{"unknown organization", func(_ *testing.T, _ *Blockchain, tx *Transaction) { tx.OrganizationID = "unknown" }, identity.ErrOrganizationNotFound},
		{"claims another organization", func(t *testing.T, c *Blockchain, tx *Transaction) {
			r := c.registry.(*identity.MemoryRegistry)
			if err := r.RegisterOrganization(identity.Organization{ID: "hospital-b", Name: "Hospital B", Status: identity.OrganizationActive}); err != nil {
				t.Fatal(err)
			}
			tx.OrganizationID = "hospital-b"
		}, ErrOrganizationMismatch},
		{"public key mismatch", func(t *testing.T, _ *Blockchain, tx *Transaction) {
			_, attackerKey, err := trustcrypto.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			if err := tx.Sign(attackerKey); err != nil {
				t.Fatal(err)
			}
		}, ErrPublicKeyMismatch},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, privateKey := newIdentityChain(t, identity.RoleDoctor, identity.ActorActive)
			tx := signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)
			tt.mutate(t, chain, &tx)
			// Re-sign claims where the test did not intentionally replace the key.
			if tt.want != ErrPublicKeyMismatch {
				if err := tx.Sign(privateKey); err != nil {
					t.Fatal(err)
				}
			}
			_, err := chain.AddBlock(testTime, []Transaction{tx})
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestInactiveActorsRejected(t *testing.T) {
	for _, status := range []identity.ActorStatus{identity.ActorSuspended, identity.ActorRevoked} {
		t.Run(string(status), func(t *testing.T) {
			chain, privateKey := newIdentityChain(t, identity.RoleDoctor, status)
			_, err := chain.AddBlock(testTime, []Transaction{signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)})
			if !errors.Is(err, ErrActorInactive) {
				t.Fatalf("error = %v, want inactive actor", err)
			}
		})
	}
}

func TestForgedActorIdentityRejected(t *testing.T) {
	chain, _ := newIdentityChain(t, identity.RoleDoctor, identity.ActorActive)
	_, attackerPrivateKey, err := trustcrypto.GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	forged := signedTransaction(t, attackerPrivateKey, "actor-a", "hospital-a", RecordCommitted)
	if !forged.VerifySignature() {
		t.Fatal("attack setup must have a cryptographically valid signature")
	}
	if _, err := chain.AddBlock(testTime, []Transaction{forged}); !errors.Is(err, ErrPublicKeyMismatch) {
		t.Fatalf("error = %v, want public key mismatch", err)
	}
}

func TestValidBlockLinkageAndChain(t *testing.T) {
	chain, privateKey := newIdentityChain(t, identity.RoleDoctor, identity.ActorActive)
	tx := signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)
	first, err := chain.AddBlock(testTime, []Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}
	second, err := chain.AddBlock(testTime.Add(time.Minute), []Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}
	if second.PreviousHash != first.Hash {
		t.Fatal("blocks are not linked")
	}
	if err := chain.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidationDetectsTampering(t *testing.T) {
	tests := []struct {
		name   string
		tamper func(*Blockchain)
		want   string
	}{
		{"modified transaction", func(c *Blockchain) { c.blocks[1].Transactions[0].ResourceID = "altered" }, "hash mismatch"},
		{"modified block", func(c *Blockchain) { c.blocks[1].Timestamp = c.blocks[1].Timestamp.Add(time.Second) }, "hash mismatch"},
		{"broken linkage", func(c *Blockchain) {
			c.blocks[2].PreviousHash[0] ^= 0xff
			c.blocks[2].Hash = c.blocks[2].CalculateHash()
		}, "previous hash mismatch"},
		{"invalid signature", func(c *Blockchain) {
			c.blocks[1].Transactions[0].Signature[0] ^= 0xff
			c.blocks[1].Hash = c.blocks[1].CalculateHash()
		}, "invalid signature"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chain, privateKey := newIdentityChain(t, identity.RoleDoctor, identity.ActorActive)
			tx := signedTransaction(t, privateKey, "actor-a", "hospital-a", RecordCommitted)
			if _, err := chain.AddBlock(testTime, []Transaction{tx}); err != nil {
				t.Fatal(err)
			}
			if _, err := chain.AddBlock(testTime.Add(time.Minute), []Transaction{tx}); err != nil {
				t.Fatal(err)
			}
			tt.tamper(chain)
			if err := chain.Validate(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.want)
			}
		})
	}
}
