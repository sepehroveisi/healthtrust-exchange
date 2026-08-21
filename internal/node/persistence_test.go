package node

import (
	"context"
	"errors"
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
)

type failingBlockStore struct{}

func (failingBlockStore) SaveBlock(context.Context, blockchain.Block) error {
	return errors.New("database unavailable")
}

func TestPersistenceFailureDoesNotMutateChain(t *testing.T) {
	n, key := testNode(t)
	n.SetBlockStore(failingBlockStore{})
	tx := testTransaction(t, key)
	if err := n.SubmitTransaction(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := n.CreateBlock(context.Background(), nodeTestTime); err == nil {
		t.Fatal("expected persistence failure")
	}
	latest, _ := n.Chain.GetLatestBlock()
	if latest.Height != 0 {
		t.Fatalf("chain mutated to height %d", latest.Height)
	}
	if n.Pool.Len() != 1 {
		t.Fatal("pending transaction removed after persistence failure")
	}
}
