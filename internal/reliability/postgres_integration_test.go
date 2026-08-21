package reliability_test

import (
	"context"
	"crypto/sha256"
	"errors"
	pg "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/reliability"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func setup(t *testing.T) (context.Context, *pg.Store) {
	url := os.Getenv("HEALTHTRUST_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("real PostgreSQL URL required")
	}
	ctx := context.Background()
	s, err := pg.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	p, _ := pgxpool.New(ctx, url)
	_, err = p.Exec(ctx, `TRUNCATE idempotency_keys,consumed_requester_proofs,consumed_peer_requests`)
	p.Close()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	return ctx, s
}
func TestConcurrentIdempotencyAndConflict(t *testing.T) {
	ctx, s := setup(t)
	hash := reliability.RequestHash([]byte("same"))
	var owners atomic.Int32
	var pending atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := s.BeginIdempotent(ctx, "op", "key", "actor", hash)
			if r.Owner {
				owners.Add(1)
			} else if errors.Is(err, reliability.ErrOperationPending) {
				pending.Add(1)
			}
		}()
	}
	wg.Wait()
	if owners.Load() != 1 || pending.Load() != 11 {
		t.Fatalf("owners=%d pending=%d", owners.Load(), pending.Load())
	}
	if err := s.CompleteIdempotent(ctx, "op", "key", "ref"); err != nil {
		t.Fatal(err)
	}
	r, err := s.BeginIdempotent(ctx, "op", "key", "actor", hash)
	if err != nil || r.ResponseReference != "ref" {
		t.Fatal("completed retry did not return original")
	}
	different := sha256.Sum256([]byte("different"))
	if _, err := s.BeginIdempotent(ctx, "op", "key", "actor", different); !errors.Is(err, reliability.ErrIdempotencyConflict) {
		t.Fatal("different request accepted")
	}
}
func TestConcurrentReplayStores(t *testing.T) {
	ctx, s := setup(t)
	now := time.Now().UTC()
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.ConsumeRequesterProof(ctx, "proof", "doctor", "hospital", "record", now, now.Add(time.Minute)) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("proof successes=%d", successes.Load())
	}
	successes.Store(0)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.ConsumePeerRequest(ctx, "request", "node", "hospital", now, now.Add(time.Minute)) == nil {
				successes.Add(1)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatalf("peer successes=%d", successes.Load())
	}
	if err := s.ConsumePeerRequest(ctx, "request", "node", "hospital", now, now.Add(time.Minute)); !errors.Is(err, security.ErrReplayedRequest) {
		t.Fatal("peer replay not classified")
	}
}
