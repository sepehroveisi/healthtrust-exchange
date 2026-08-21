package blockchain

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

// Blockchain is an in-memory, append-only sequence of audit blocks.
type Blockchain struct {
	mu       sync.RWMutex
	blocks   []Block
	registry identity.Registry
	policy   authorization.Policy
}

// CreateGenesisBlock creates the empty first block at a caller-supplied time.
func CreateGenesisBlock(timestamp time.Time) Block {
	return NewBlock(0, timestamp, [32]byte{}, nil)
}

// New creates a blockchain containing its genesis block.
func New(genesisTimestamp time.Time, registry identity.Registry, policy authorization.Policy) *Blockchain {
	return &Blockchain{blocks: []Block{CreateGenesisBlock(genesisTimestamp)}, registry: registry, policy: policy}
}

// Recover reconstructs and validates a chain loaded from durable storage.
func Recover(blocks []Block, registry identity.Registry, policy authorization.Policy) (*Blockchain, error) {
	if len(blocks) == 0 {
		return nil, errors.New("cannot recover an empty blockchain")
	}
	chain := &Blockchain{blocks: make([]Block, len(blocks)), registry: registry, policy: policy}
	for i := range blocks {
		chain.blocks[i] = cloneBlock(blocks[i])
	}
	if err := chain.Validate(); err != nil {
		return nil, fmt.Errorf("validate recovered blockchain: %w", err)
	}
	return chain, nil
}

// PrepareBlock validates transactions and builds the next block without mutating the chain.
func (chain *Blockchain) PrepareBlock(timestamp time.Time, transactions []Transaction) (Block, error) {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	if len(chain.blocks) == 0 {
		return Block{}, errors.New("blockchain has no genesis block")
	}
	for i, tx := range transactions {
		if err := chain.validateTransaction(tx, i); err != nil {
			return Block{}, err
		}
	}
	latest := chain.blocks[len(chain.blocks)-1]
	return NewBlock(latest.Height+1, timestamp, latest.Hash, transactions), nil
}

// ValidateNextBlock validates a candidate without mutating chain state.
func (chain *Blockchain) ValidateNextBlock(block Block) error {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	return chain.validateNextBlock(block)
}

// AddBlock validates transactions and appends a linked block.
func (chain *Blockchain) AddBlock(timestamp time.Time, transactions []Transaction) (Block, error) {
	chain.mu.Lock()
	defer chain.mu.Unlock()
	if chain == nil || len(chain.blocks) == 0 {
		return Block{}, errors.New("blockchain has no genesis block")
	}
	for i, tx := range transactions {
		if err := chain.validateTransaction(tx, i); err != nil {
			return Block{}, err
		}
	}
	latest := chain.blocks[len(chain.blocks)-1]
	block := NewBlock(latest.Height+1, timestamp, latest.Hash, transactions)
	chain.blocks = append(chain.blocks, block)
	return block, nil
}

// AppendBlock independently validates and atomically appends a remotely supplied block.
func (chain *Blockchain) AppendBlock(block Block) error {
	chain.mu.Lock()
	defer chain.mu.Unlock()
	if err := chain.validateNextBlock(block); err != nil {
		return err
	}
	chain.blocks = append(chain.blocks, cloneBlock(block))
	return nil
}

func (chain *Blockchain) validateNextBlock(block Block) error {
	if len(chain.blocks) == 0 {
		return errors.New("blockchain has no genesis block")
	}
	latest := chain.blocks[len(chain.blocks)-1]
	if block.Height != latest.Height+1 {
		return fmt.Errorf("invalid block height: got %d, want %d", block.Height, latest.Height+1)
	}
	if block.PreviousHash != latest.Hash {
		return errors.New("previous hash mismatch")
	}
	if block.Hash != block.CalculateHash() {
		return errors.New("block hash mismatch")
	}
	for i, tx := range block.Transactions {
		if err := chain.validateTransaction(tx, i); err != nil {
			return err
		}
	}
	return nil
}

// ValidateTransaction validates an already-signed transaction against trusted identity and policy.
func (chain *Blockchain) ValidateTransaction(tx Transaction) error {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	return chain.validateTransaction(tx, 0)
}

// GetLatestBlock returns a defensive copy of the latest block.
func (chain *Blockchain) GetLatestBlock() (Block, error) {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	if chain == nil || len(chain.blocks) == 0 {
		return Block{}, errors.New("blockchain is empty")
	}
	return cloneBlock(chain.blocks[len(chain.blocks)-1]), nil
}

// Blocks returns a defensive snapshot of the chain.
func (chain *Blockchain) Blocks() []Block {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	if chain == nil {
		return nil
	}
	result := make([]Block, len(chain.blocks))
	for i := range chain.blocks {
		result[i] = cloneBlock(chain.blocks[i])
	}
	return result
}

// HasTransaction reports whether a deterministic audit event ID is already committed.
func (chain *Blockchain) HasTransaction(id string) bool {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	for _, block := range chain.blocks {
		for _, tx := range block.Transactions {
			if tx.ID == id {
				return true
			}
		}
	}
	return false
}

func cloneBlock(block Block) Block {
	block.Transactions = cloneTransactions(block.Transactions)
	return block
}
