package blockchain

import (
	"crypto/sha256"
	"time"
)

// Block groups signed audit transactions and links to the preceding block.
type Block struct {
	Height       uint64
	Timestamp    time.Time
	PreviousHash [sha256.Size]byte
	Transactions []Transaction
	Hash         [sha256.Size]byte
}

// NewBlock constructs a block and calculates its deterministic hash.
func NewBlock(height uint64, timestamp time.Time, previousHash [sha256.Size]byte, transactions []Transaction) Block {
	block := Block{
		Height: height, Timestamp: timestamp.UTC(), PreviousHash: previousHash,
		Transactions: cloneTransactions(transactions),
	}
	block.Hash = block.CalculateHash()
	return block
}

// CalculateHash hashes the block metadata and complete signed transactions.
func (b Block) CalculateHash() [sha256.Size]byte {
	h := sha256.New()
	writeUint64(h, b.Height)
	writeUint64(h, uint64(b.Timestamp.UTC().UnixNano()))
	writeBytes(h, b.PreviousHash[:])
	writeUint64(h, uint64(len(b.Transactions)))
	for _, tx := range b.Transactions {
		txHash := tx.Hash()
		writeBytes(h, txHash[:])
		writeBytes(h, tx.Signature)
	}
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

func cloneTransactions(transactions []Transaction) []Transaction {
	result := make([]Transaction, len(transactions))
	copy(result, transactions)
	for i := range result {
		result[i].PayloadHash = append([]byte(nil), transactions[i].PayloadHash...)
		result[i].PublicKey = append([]byte(nil), transactions[i].PublicKey...)
		result[i].Signature = append([]byte(nil), transactions[i].Signature...)
	}
	return result
}
