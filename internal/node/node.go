package node

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

type Transport interface {
	BroadcastBlock(context.Context, []Peer, blockchain.Block) error
	FetchBlocks(context.Context, Peer, uint64) ([]blockchain.Block, error)
}

type BlockStore interface {
	SaveBlock(context.Context, blockchain.Block) error
}

type Node struct {
	ID             string
	OrganizationID string
	Chain          *blockchain.Blockchain
	Registry       identity.Registry
	Policy         authorization.Policy
	Pool           *TransactionPool
	Peers          []Peer
	Transport      Transport
	Store          BlockStore
	Logger         *slog.Logger
	mutationMu     sync.Mutex
}

func (n *Node) SetBlockStore(store BlockStore) { n.Store = store }

func (n *Node) HasTransaction(id string) bool { return n.Chain.HasTransaction(id) }
func (n *Node) Blocks() []blockchain.Block    { return n.Chain.Blocks() }

func New(id, organizationID string, chain *blockchain.Blockchain, registry identity.Registry, policy authorization.Policy, peers []Peer, transport Transport, logger *slog.Logger) *Node {
	if logger == nil {
		logger = slog.Default()
	}
	return &Node{ID: id, OrganizationID: organizationID, Chain: chain, Registry: registry, Policy: policy, Pool: NewTransactionPool(), Peers: append([]Peer(nil), peers...), Transport: transport, Logger: logger}
}

func (n *Node) SubmitTransaction(tx blockchain.Transaction) error {
	if err := n.Chain.ValidateTransaction(tx); err != nil {
		n.Logger.Warn("transaction_rejected", "node_id", n.ID, "organization_id", n.OrganizationID, "transaction_id", tx.ID, "event_type", tx.Type, "resource_id", tx.ResourceID, "error", err)
		return err
	}
	if err := n.Pool.Add(tx); err != nil {
		return err
	}
	txHash := tx.Hash()
	n.Logger.Info("transaction_accepted", "node_id", n.ID, "organization_id", n.OrganizationID, "transaction_id", tx.ID, "transaction_hash", fmt.Sprintf("%x", txHash), "event_type", tx.Type, "resource_id", tx.ResourceID, "payload_hash", fmt.Sprintf("%x", tx.PayloadHash))
	return nil
}

func (n *Node) CreateBlock(ctx context.Context, timestamp time.Time) (blockchain.Block, error) {
	n.mutationMu.Lock()
	defer n.mutationMu.Unlock()
	transactions := n.Pool.Snapshot()
	if len(transactions) == 0 {
		return blockchain.Block{}, errors.New("transaction pool is empty")
	}
	block, err := n.Chain.PrepareBlock(timestamp, transactions)
	if err != nil {
		return blockchain.Block{}, err
	}
	if n.Store != nil {
		if err := n.Store.SaveBlock(ctx, block); err != nil {
			return blockchain.Block{}, err
		}
	}
	if err := n.Chain.AppendBlock(block); err != nil {
		return blockchain.Block{}, err
	}
	n.Pool.Remove(transactions)
	n.Logger.Info("block_created", blockLogFields(n, block)...)
	if n.Transport != nil {
		if err := n.Transport.BroadcastBlock(ctx, n.Peers, block); err != nil {
			n.Logger.Warn("block_propagation_deferred", "height", block.Height, "error", err)
		}
	}
	return block, nil
}

func (n *Node) ReceiveBlock(block blockchain.Block) error {
	return n.ReceiveBlockContext(context.Background(), block)
}

func (n *Node) ReceiveBlockContext(ctx context.Context, block blockchain.Block) error {
	n.mutationMu.Lock()
	defer n.mutationMu.Unlock()
	n.Logger.Info("block_received", blockLogFields(n, block)...)
	if err := n.Chain.ValidateNextBlock(block); err != nil {
		n.Logger.Warn("block_rejected", append(blockLogFields(n, block), "error", err)...)
		return err
	}
	if n.Store != nil {
		if err := n.Store.SaveBlock(ctx, block); err != nil {
			return err
		}
	}
	if err := n.Chain.AppendBlock(block); err != nil {
		return err
	}
	n.Pool.Remove(block.Transactions)
	n.Logger.Info("block_accepted", append(blockLogFields(n, block), "validation", "passed")...)
	return nil
}

func blockLogFields(n *Node, block blockchain.Block) []any {
	transactionIDs := make([]string, 0, len(block.Transactions))
	eventTypes := make([]string, 0, len(block.Transactions))
	for _, tx := range block.Transactions {
		transactionIDs = append(transactionIDs, tx.ID)
		eventTypes = append(eventTypes, string(tx.Type))
	}
	return []any{"node_id", n.ID, "organization_id", n.OrganizationID, "height", block.Height, "block_hash", fmt.Sprintf("%x", block.Hash), "previous_hash", fmt.Sprintf("%x", block.PreviousHash), "transaction_count", len(block.Transactions), "transaction_ids", transactionIDs, "event_types", eventTypes}
}

func (n *Node) Sync(ctx context.Context, peer Peer) error {
	if n.Transport == nil {
		return errors.New("transport is not configured")
	}
	n.Logger.Info("sync_started", "peer", peer.ID)
	latest, err := n.Chain.GetLatestBlock()
	if err != nil {
		return err
	}
	blocks, err := n.Transport.FetchBlocks(ctx, peer, latest.Height+1)
	if err != nil {
		return err
	}
	for _, block := range blocks {
		if err := n.ReceiveBlockContext(ctx, block); err != nil {
			return err
		}
	}
	n.Logger.Info("sync_completed", "peer", peer.ID, "imported", len(blocks))
	return nil
}
