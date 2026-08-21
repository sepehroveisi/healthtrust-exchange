package node

import (
	"errors"
	"sync"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
)

var ErrDuplicateTransaction = errors.New("duplicate transaction")

type TransactionPool struct {
	mu           sync.Mutex
	transactions map[string]blockchain.Transaction
}

func NewTransactionPool() *TransactionPool {
	return &TransactionPool{transactions: make(map[string]blockchain.Transaction)}
}

func (p *TransactionPool) Add(tx blockchain.Transaction) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, exists := p.transactions[tx.ID]; exists {
		return ErrDuplicateTransaction
	}
	p.transactions[tx.ID] = tx
	return nil
}

func (p *TransactionPool) Snapshot() []blockchain.Transaction {
	p.mu.Lock()
	defer p.mu.Unlock()
	result := make([]blockchain.Transaction, 0, len(p.transactions))
	for _, tx := range p.transactions {
		result = append(result, tx)
	}
	return result
}

func (p *TransactionPool) Remove(transactions []blockchain.Transaction) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, tx := range transactions {
		delete(p.transactions, tx.ID)
	}
}

func (p *TransactionPool) Len() int { p.mu.Lock(); defer p.mu.Unlock(); return len(p.transactions) }
