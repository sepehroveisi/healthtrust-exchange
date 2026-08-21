package recovery

import (
	"context"
	"crypto/sha256"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	pgstore "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
)

type memoryStore struct {
	mu            sync.Mutex
	organizations map[string]identity.Organization
	actors        map[string]identity.Actor
	blocks        []blockchain.Block
	saveErr       error
}

func newMemoryStore() *memoryStore {
	return &memoryStore{organizations: map[string]identity.Organization{}, actors: map[string]identity.Actor{}}
}
func (s *memoryStore) SaveOrganization(_ context.Context, v identity.Organization) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.organizations[v.ID] = v
	return nil
}
func (s *memoryStore) GetOrganization(_ context.Context, id string) (identity.Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.organizations[id]
	if !ok {
		return v, identity.ErrOrganizationNotFound
	}
	return v, nil
}
func (s *memoryStore) ListOrganizations(context.Context) ([]identity.Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := make([]identity.Organization, 0, len(s.organizations))
	for _, v := range s.organizations {
		r = append(r, v)
	}
	return r, nil
}
func (s *memoryStore) SaveActor(_ context.Context, v identity.Actor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	v.PublicKey = append([]byte(nil), v.PublicKey...)
	s.actors[v.ID] = v
	return nil
}
func (s *memoryStore) GetActor(_ context.Context, id string) (identity.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.actors[id]
	if !ok {
		return v, identity.ErrActorNotFound
	}
	v.PublicKey = append([]byte(nil), v.PublicKey...)
	return v, nil
}
func (s *memoryStore) ListActors(context.Context) ([]identity.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r := make([]identity.Actor, 0, len(s.actors))
	for _, v := range s.actors {
		v.PublicKey = append([]byte(nil), v.PublicKey...)
		r = append(r, v)
	}
	return r, nil
}
func (s *memoryStore) SaveBlock(_ context.Context, v blockchain.Block) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.saveErr != nil {
		return s.saveErr
	}
	for _, b := range s.blocks {
		if b.Height == v.Height {
			return errors.New("duplicate block")
		}
	}
	s.blocks = append(s.blocks, v)
	return nil
}
func (s *memoryStore) GetBlock(_ context.Context, h uint64) (blockchain.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, v := range s.blocks {
		if v.Height == h {
			return v, nil
		}
	}
	return blockchain.Block{}, pgstore.ErrBlockNotFound
}
func (s *memoryStore) GetBlocksFrom(_ context.Context, h uint64) ([]blockchain.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r []blockchain.Block
	for _, v := range s.blocks {
		if v.Height >= h {
			r = append(r, v)
		}
	}
	return r, nil
}
func (s *memoryStore) GetLatestBlock(ctx context.Context) (blockchain.Block, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.blocks) == 0 {
		return blockchain.Block{}, pgstore.ErrBlockNotFound
	}
	return s.blocks[len(s.blocks)-1], nil
}

func TestFirstAndSecondStartupReuseGenesisAndIdentities(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	policy := authorization.HealthcarePolicy{}
	first, err := Load(ctx, store, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Load(ctx, store, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := first.Chain.GetLatestBlock()
	b, _ := second.Chain.GetLatestBlock()
	if a.Hash != b.Hash || a.Height != 0 {
		t.Fatal("genesis was not reused")
	}
	actor, err := second.Registry.GetActor("doctor-a")
	if err != nil || len(actor.PublicKey) != 32 {
		t.Fatal("actor identity was not recovered")
	}
}

func TestRestartPreservesChainAndSignatureValidation(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	policy := authorization.HealthcarePolicy{}
	state, err := Load(ctx, store, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	_, keys, _ := demodata.Registry()
	payload := sha256.Sum256([]byte("off-chain"))
	tx := blockchain.Transaction{ID: "persisted-1", Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record-1", PayloadHash: payload[:], Timestamp: genesis.Add(time.Minute)}
	if err := tx.Sign(keys["doctor-a"]); err != nil {
		t.Fatal(err)
	}
	block, err := state.Chain.PrepareBlock(tx.Timestamp, []blockchain.Transaction{tx})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveBlock(ctx, block); err != nil {
		t.Fatal(err)
	}
	restarted, err := Load(ctx, store, policy, genesis, true)
	if err != nil {
		t.Fatal(err)
	}
	latest, _ := restarted.Chain.GetLatestBlock()
	if latest.Height != 1 || latest.Hash != block.Hash {
		t.Fatal("chain height/hash not recovered")
	}
	if err := restarted.Chain.ValidateTransaction(tx); err != nil {
		t.Fatalf("previously signed transaction failed after identity reload: %v", err)
	}
}

func TestCorruptedPersistedChainFailsRecovery(t *testing.T) {
	ctx := context.Background()
	store := newMemoryStore()
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := Load(ctx, store, authorization.HealthcarePolicy{}, genesis, true); err != nil {
		t.Fatal(err)
	}
	store.blocks[0].Hash[0] ^= 1
	if _, err := Load(ctx, store, authorization.HealthcarePolicy{}, genesis, true); err == nil {
		t.Fatal("corrupted persisted chain accepted")
	}
}
