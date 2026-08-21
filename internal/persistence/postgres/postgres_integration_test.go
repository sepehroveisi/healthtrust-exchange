package postgres_test

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"io"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
	pgstore "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/recovery"
	"github.com/jackc/pgx/v5/pgxpool"
)

type sourceTransport struct{ blocks []blockchain.Block }

func (s sourceTransport) BroadcastBlock(context.Context, []node.Peer, blockchain.Block) error {
	return nil
}
func (s sourceTransport) FetchBlocks(_ context.Context, _ node.Peer, from uint64) ([]blockchain.Block, error) {
	if from >= uint64(len(s.blocks)) {
		return nil, nil
	}
	return s.blocks[from:], nil
}

func integrationStore(t *testing.T) (context.Context, *pgstore.Store) {
	t.Helper()
	databaseURL := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set HEALTHTRUST_TEST_DATABASE_URL to run PostgreSQL integration tests")
	}
	ctx := context.Background()
	store, err := pgstore.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		store.Close()
		t.Fatal(err)
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		store.Close()
		t.Fatal(err)
	}
	if _, err := admin.Exec(ctx, `TRUNCATE transactions, blocks, actors, organizations CASCADE`); err != nil {
		admin.Close()
		store.Close()
		t.Fatal(err)
	}
	admin.Close()
	t.Cleanup(store.Close)
	return ctx, store
}

func seedStore(t *testing.T, ctx context.Context, store *pgstore.Store) map[string]ed25519.PrivateKey {
	t.Helper()
	registry, keys, err := demodata.Registry()
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"hospital-a", "hospital-b"} {
		v, _ := registry.GetOrganization(id)
		if err := store.SaveOrganization(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"doctor-a", "doctor-b", "patient-p"} {
		v, _ := registry.GetActor(id)
		if err := store.SaveActor(ctx, v); err != nil {
			t.Fatal(err)
		}
	}
	result := map[string]ed25519.PrivateKey{}
	for id, key := range keys {
		result[id] = key
	}
	return result
}

func TestIdentityRoundTrip(t *testing.T) {
	ctx, store := integrationStore(t)
	keys := seedStore(t, ctx, store)
	organization, err := store.GetOrganization(ctx, "hospital-a")
	if err != nil || organization.Name != "Hospital A" {
		t.Fatalf("organization round trip: %v %+v", err, organization)
	}
	actor, err := store.GetActor(ctx, "doctor-a")
	if err != nil {
		t.Fatal(err)
	}
	expected := keys["doctor-a"][32:]
	if !bytes.Equal(actor.PublicKey, expected) {
		t.Fatal("actor public key bytes changed during round trip")
	}
	if actor.Role != identity.RoleDoctor || actor.Status != identity.ActorActive {
		t.Fatal("actor role/status changed")
	}
}

func TestBlockRoundTripOrderingHashAndSignature(t *testing.T) {
	ctx, store := integrationStore(t)
	keys := seedStore(t, ctx, store)
	genesis := blockchain.CreateGenesisBlock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := store.SaveBlock(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	var transactions []blockchain.Transaction
	for i, id := range []string{"first", "second"} {
		payload := sha256.Sum256([]byte(id))
		tx := blockchain.Transaction{ID: id, Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record-" + id, PayloadHash: payload[:], Timestamp: genesis.Timestamp.Add(time.Duration(i+1) * time.Minute)}
		if err := tx.Sign(keys["doctor-a"]); err != nil {
			t.Fatal(err)
		}
		transactions = append(transactions, tx)
	}
	block := blockchain.NewBlock(1, genesis.Timestamp.Add(time.Hour), genesis.Hash, transactions)
	if err := store.SaveBlock(ctx, block); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.GetBlock(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hash != block.Hash || loaded.CalculateHash() != block.Hash {
		t.Fatal("block hash changed after reload")
	}
	if len(loaded.Transactions) != 2 || loaded.Transactions[0].ID != "first" || loaded.Transactions[1].ID != "second" {
		t.Fatal("transaction order changed")
	}
	for _, tx := range loaded.Transactions {
		if !tx.VerifySignature() {
			t.Fatal("signature invalid after reload")
		}
	}
	blocks, err := store.GetBlocksFrom(ctx, 0)
	if err != nil || len(blocks) != 2 || blocks[0].Height != 0 || blocks[1].Height != 1 {
		t.Fatalf("blocks loaded out of order: %v %+v", err, blocks)
	}
}

func TestAtomicRollbackAndConstraints(t *testing.T) {
	ctx, store := integrationStore(t)
	keys := seedStore(t, ctx, store)
	genesis := blockchain.CreateGenesisBlock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if err := store.SaveBlock(ctx, genesis); err != nil {
		t.Fatal(err)
	}
	payload := sha256.Sum256([]byte("duplicate"))
	tx := blockchain.Transaction{ID: "duplicate", Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record", PayloadHash: payload[:], Timestamp: genesis.Timestamp.Add(time.Minute)}
	if err := tx.Sign(keys["doctor-a"]); err != nil {
		t.Fatal(err)
	}
	block := blockchain.NewBlock(1, tx.Timestamp, genesis.Hash, []blockchain.Transaction{tx, tx})
	if err := store.SaveBlock(ctx, block); err == nil {
		t.Fatal("duplicate transaction ID accepted")
	}
	if _, err := store.GetBlock(ctx, 1); !errors.Is(err, pgstore.ErrBlockNotFound) {
		t.Fatalf("partial block survived rollback: %v", err)
	}
	tx.ID = "foreign-key"
	tx.ActorID = "unknown"
	if err := tx.Sign(keys["doctor-a"]); err != nil {
		t.Fatal(err)
	}
	block = blockchain.NewBlock(1, tx.Timestamp, genesis.Hash, []blockchain.Transaction{tx})
	if err := store.SaveBlock(ctx, block); err == nil {
		t.Fatal("foreign-key violation accepted")
	}
	if err := store.SaveBlock(ctx, genesis); err == nil {
		t.Fatal("duplicate block accepted")
	}
}

func TestUnavailableDatabaseFailsClearly(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	store, err := pgstore.Open(ctx, "postgres://healthtrust:healthtrust@127.0.0.1:1/healthtrust?sslmode=disable")
	if store != nil {
		store.Close()
	}
	if err == nil {
		t.Fatal("unavailable database unexpectedly connected")
	}
}

func TestHospitalBRestartAndSync(t *testing.T) {
	databaseURLA := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL")
	databaseURLB := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL_B")
	if databaseURLA == "" || databaseURLB == "" {
		t.Skip("set both HEALTHTRUST_TEST_DATABASE_URL and HEALTHTRUST_TEST_DATABASE_URL_B")
	}
	ctx := context.Background()
	storeA, err := pgstore.Open(ctx, databaseURLA)
	if err != nil {
		t.Fatal(err)
	}
	defer storeA.Close()
	storeB, err := pgstore.Open(ctx, databaseURLB)
	if err != nil {
		t.Fatal(err)
	}
	defer storeB.Close()
	for _, store := range []*pgstore.Store{storeA, storeB} {
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, url := range []string{databaseURLA, databaseURLB} {
		pool, err := pgxpool.New(ctx, url)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `TRUNCATE transactions, blocks, actors, organizations CASCADE`); err != nil {
			pool.Close()
			t.Fatal(err)
		}
		pool.Close()
	}
	policy := authorization.HealthcarePolicy{}
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	stateA, err := recovery.Load(ctx, storeA, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	stateB, err := recovery.Load(ctx, storeB, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodeA := node.New("a", "hospital-a", stateA.Chain, stateA.Registry, policy, nil, nil, logger)
	nodeA.SetBlockStore(storeA)
	nodeB := node.New("b", "hospital-b", stateB.Chain, stateB.Registry, policy, nil, nil, logger)
	nodeB.SetBlockStore(storeB)
	_, keys, _ := demodata.Registry()
	makeTransaction := func(id string, at time.Time) blockchain.Transaction {
		payload := sha256.Sum256([]byte(id))
		tx := blockchain.Transaction{ID: id, Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record-" + id, PayloadHash: payload[:], Timestamp: at}
		if err := tx.Sign(keys["doctor-a"]); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	tx1 := makeTransaction("restart-1", genesis.Add(time.Minute))
	if err := nodeA.SubmitTransaction(tx1); err != nil {
		t.Fatal(err)
	}
	block1, err := nodeA.CreateBlock(ctx, tx1.Timestamp)
	if err != nil {
		t.Fatal(err)
	}
	if err := nodeB.ReceiveBlockContext(ctx, block1); err != nil {
		t.Fatal(err)
	}
	tx2 := makeTransaction("restart-2", genesis.Add(2*time.Minute))
	if err := nodeA.SubmitTransaction(tx2); err != nil {
		t.Fatal(err)
	}
	if _, err := nodeA.CreateBlock(ctx, tx2.Timestamp); err != nil {
		t.Fatal(err)
	}
	recoveredB, err := recovery.Load(ctx, storeB, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := recoveredB.Chain.GetLatestBlock()
	if before.Height != 1 {
		t.Fatalf("Hospital B recovered height = %d", before.Height)
	}
	restartedB := node.New("b", "hospital-b", recoveredB.Chain, recoveredB.Registry, policy, nil, sourceTransport{blocks: nodeA.Chain.Blocks()}, logger)
	restartedB.SetBlockStore(storeB)
	if err := restartedB.Sync(ctx, node.Peer{ID: "a"}); err != nil {
		t.Fatal(err)
	}
	finalA, err := recovery.Load(ctx, storeA, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	finalB, err := recovery.Load(ctx, storeB, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	latestA, _ := finalA.Chain.GetLatestBlock()
	latestB, _ := finalB.Chain.GetLatestBlock()
	if latestA.Height != 2 || latestB.Height != 2 || latestA.Hash != latestB.Hash {
		t.Fatalf("restart sync failed: A=%d B=%d", latestA.Height, latestB.Height)
	}
}
