package node

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	trustcrypto "github.com/healthtrust-exchange/healthtrust-exchange/internal/cryptography"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

func TestBlockchainLogsContainRuntimeEvidence(t *testing.T) {
	n, key := testNode(t)
	var output bytes.Buffer
	n.Logger = slog.New(slog.NewJSONHandler(&output, nil))
	tx := testTransaction(t, key)
	if err := n.SubmitTransaction(tx); err != nil {
		t.Fatal(err)
	}
	block, err := n.CreateBlock(context.Background(), nodeTestTime)
	if err != nil {
		t.Fatal(err)
	}
	logText := output.String()
	for _, field := range []string{`"msg":"transaction_accepted"`, `"node_id":"node-a"`, `"event_type":"RecordCommitted"`, `"resource_id":"record-1"`, `"transaction_hash":`, `"payload_hash":`, `"msg":"block_created"`, `"block_hash":`, `"transaction_ids":["tx-1"]`, `"height":1`} {
		if !strings.Contains(logText, field) {
			t.Fatalf("log missing %s: %s", field, logText)
		}
	}
	if block.Height != 1 {
		t.Fatalf("height = %d", block.Height)
	}
}

var nodeTestTime = time.Date(2026, 1, 1, 1, 0, 0, 0, time.UTC)

type fakeTransport struct {
	blocks []blockchain.Block
	err    error
}

func (f fakeTransport) BroadcastBlock(context.Context, []Peer, blockchain.Block) error { return f.err }
func (f fakeTransport) FetchBlocks(_ context.Context, _ Peer, from uint64) ([]blockchain.Block, error) {
	if f.err != nil {
		return nil, f.err
	}
	if from >= uint64(len(f.blocks)) {
		return nil, nil
	}
	return f.blocks[from:], nil
}

func testNode(t *testing.T) (*Node, ed25519.PrivateKey) {
	t.Helper()
	seed := sha256.Sum256([]byte("shared-node-test-doctor-a"))
	privateKey := ed25519.NewKeyFromSeed(seed[:])
	publicKey := privateKey.Public().(ed25519.PublicKey)
	registry := identity.NewMemoryRegistry()
	if err := registry.RegisterOrganization(identity.Organization{ID: "hospital-a", Name: "Hospital A", Status: identity.OrganizationActive}); err != nil {
		t.Fatal(err)
	}
	if err := registry.RegisterActor(identity.Actor{ID: "doctor-a", OrganizationID: "hospital-a", Role: identity.RoleDoctor, PublicKey: publicKey, Status: identity.ActorActive}); err != nil {
		t.Fatal(err)
	}
	policy := authorization.HealthcarePolicy{}
	chain := blockchain.New(nodeTestTime.Add(-time.Hour), registry, policy)
	return New("node-a", "hospital-a", chain, registry, policy, nil, nil, slog.New(slog.NewTextHandler(io.Discard, nil))), privateKey
}

func testTransaction(t *testing.T, privateKey ed25519.PrivateKey) blockchain.Transaction {
	t.Helper()
	payload := sha256.Sum256([]byte("fictional"))
	tx := blockchain.Transaction{ID: "tx-1", Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record-1", PayloadHash: payload[:], Timestamp: nodeTestTime}
	if err := tx.Sign(privateKey); err != nil {
		t.Fatal(err)
	}
	return tx
}

func validBlock(t *testing.T) (*Node, blockchain.Block, ed25519.PrivateKey) {
	t.Helper()
	source, key := testNode(t)
	tx := testTransaction(t, key)
	block, err := source.Chain.AddBlock(nodeTestTime, []blockchain.Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}
	return source, block, key
}

func TestReceiveBlockValidation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *blockchain.Block, ed25519.PrivateKey)
		valid  bool
	}{
		{"valid remote block", func(_ *testing.T, _ *blockchain.Block, _ ed25519.PrivateKey) {}, true},
		{"invalid previous hash", func(_ *testing.T, b *blockchain.Block, _ ed25519.PrivateKey) {
			b.PreviousHash[0] ^= 1
			b.Hash = b.CalculateHash()
		}, false},
		{"modified hash", func(_ *testing.T, b *blockchain.Block, _ ed25519.PrivateKey) { b.Hash[0] ^= 1 }, false},
		{"invalid signature", func(_ *testing.T, b *blockchain.Block, _ ed25519.PrivateKey) {
			b.Transactions[0].Signature[0] ^= 1
			b.Hash = b.CalculateHash()
		}, false},
		{"unknown actor", func(t *testing.T, b *blockchain.Block, key ed25519.PrivateKey) {
			b.Transactions[0].ActorID = "unknown"
			if err := b.Transactions[0].Sign(key); err != nil {
				t.Fatal(err)
			}
			b.Hash = b.CalculateHash()
		}, false},
		{"public key mismatch", func(t *testing.T, b *blockchain.Block, _ ed25519.PrivateKey) {
			_, attacker, err := trustcrypto.GenerateKeyPair()
			if err != nil {
				t.Fatal(err)
			}
			if err := b.Transactions[0].Sign(attacker); err != nil {
				t.Fatal(err)
			}
			b.Hash = b.CalculateHash()
		}, false},
		{"unauthorized transaction", func(t *testing.T, b *blockchain.Block, key ed25519.PrivateKey) {
			b.Transactions[0].Type = blockchain.ConsentGranted
			if err := b.Transactions[0].Sign(key); err != nil {
				t.Fatal(err)
			}
			b.Hash = b.CalculateHash()
		}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, _ := testNode(t)
			_, block, key := validBlock(t)
			tt.mutate(t, &block, key)
			err := target.ReceiveBlock(block)
			if tt.valid && err != nil {
				t.Fatalf("ReceiveBlock() error = %v", err)
			}
			if !tt.valid && err == nil {
				t.Fatal("invalid block accepted")
			}
			latest, _ := target.Chain.GetLatestBlock()
			expected := uint64(0)
			if tt.valid {
				expected = 1
			}
			if latest.Height != expected {
				t.Fatalf("height = %d, want %d", latest.Height, expected)
			}
		})
	}
}

func TestDuplicateBlockHandledSafely(t *testing.T) {
	target, _ := testNode(t)
	_, block, _ := validBlock(t)
	if err := target.ReceiveBlock(block); err != nil {
		t.Fatal(err)
	}
	if err := target.ReceiveBlock(block); err == nil {
		t.Fatal("duplicate block accepted")
	}
	latest, _ := target.Chain.GetLatestBlock()
	if latest.Height != 1 {
		t.Fatalf("height = %d", latest.Height)
	}
}

func TestSynchronization(t *testing.T) {
	for _, count := range []int{1, 3} {
		t.Run(string(rune('0'+count))+" blocks behind", func(t *testing.T) {
			source, key := testNode(t)
			for i := 0; i < count; i++ {
				tx := testTransaction(t, key)
				tx.ID = string(rune('a' + i))
				if err := tx.Sign(key); err != nil {
					t.Fatal(err)
				}
				if _, err := source.Chain.AddBlock(nodeTestTime.Add(time.Duration(i)*time.Minute), []blockchain.Transaction{tx}); err != nil {
					t.Fatal(err)
				}
			}
			target, _ := testNode(t)
			target.Transport = fakeTransport{blocks: source.Chain.Blocks()}
			if err := target.Sync(context.Background(), Peer{ID: "source"}); err != nil {
				t.Fatal(err)
			}
			sourceLatest, _ := source.Chain.GetLatestBlock()
			targetLatest, _ := target.Chain.GetLatestBlock()
			if sourceLatest.Hash != targetLatest.Hash {
				t.Fatal("chains did not converge")
			}
		})
	}
}

func TestInvalidSyncDoesNotModifyValidChain(t *testing.T) {
	source, _, _ := validBlock(t)
	blocks := source.Chain.Blocks()
	blocks[1].Hash[0] ^= 1
	target, _ := testNode(t)
	target.Transport = fakeTransport{blocks: blocks}
	before, _ := target.Chain.GetLatestBlock()
	if err := target.Sync(context.Background(), Peer{ID: "source"}); err == nil {
		t.Fatal("invalid sync accepted")
	}
	after, _ := target.Chain.GetLatestBlock()
	if before.Hash != after.Hash {
		t.Fatal("invalid sync modified chain")
	}
}

func TestTransactionPoolAdmission(t *testing.T) {
	n, key := testNode(t)
	tx := testTransaction(t, key)
	if err := n.SubmitTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if n.Pool.Len() != 1 {
		t.Fatal("valid transaction not pooled")
	}
	if err := n.SubmitTransaction(tx); !errors.Is(err, ErrDuplicateTransaction) {
		t.Fatalf("duplicate error = %v", err)
	}
	tx.Signature[0] ^= 1
	tx.ID = "invalid"
	if err := n.SubmitTransaction(tx); err == nil {
		t.Fatal("invalid transaction pooled")
	}
	if n.Pool.Len() != 1 {
		t.Fatal("invalid transaction changed pool")
	}
}

func TestForgedIdentityRejectedLocally(t *testing.T) {
	n, _ := testNode(t)
	_, attacker, _ := trustcrypto.GenerateKeyPair()
	forged := testTransaction(t, attacker)
	if !forged.VerifySignature() {
		t.Fatal("attack setup signature invalid")
	}
	if err := n.SubmitTransaction(forged); !errors.Is(err, blockchain.ErrPublicKeyMismatch) {
		t.Fatalf("error = %v", err)
	}
}

func TestCreateBlockSucceedsWhenBroadcastIsDeferred(t *testing.T) {
	n, key := testNode(t)
	n.Transport = fakeTransport{err: errors.New("peer offline")}
	if err := n.SubmitTransaction(testTransaction(t, key)); err != nil {
		t.Fatal(err)
	}
	block, err := n.CreateBlock(context.Background(), nodeTestTime)
	if err != nil {
		t.Fatalf("CreateBlock() reported a committed block as failed: %v", err)
	}
	if block.Height != 1 {
		t.Fatalf("block height = %d, want 1", block.Height)
	}
	latest, _ := n.Chain.GetLatestBlock()
	if latest.Hash != block.Hash || n.Pool.Len() != 0 {
		t.Fatal("committed block state is inconsistent")
	}
}
