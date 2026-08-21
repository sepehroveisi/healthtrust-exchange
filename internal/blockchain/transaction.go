package blockchain

import (
	"crypto/ed25519"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	trustcrypto "github.com/healthtrust-exchange/healthtrust-exchange/internal/cryptography"
)

// TransactionType identifies a healthcare audit event. It is not clinical data.
type TransactionType string

const (
	RecordCommitted TransactionType = "RecordCommitted"
	ConsentGranted  TransactionType = "ConsentGranted"
	ConsentRevoked  TransactionType = "ConsentRevoked"
	RecordAccessed  TransactionType = "RecordAccessed"
	ReferralCreated TransactionType = "ReferralCreated"
)

var validTransactionTypes = map[TransactionType]struct{}{
	RecordCommitted: {}, ConsentGranted: {}, ConsentRevoked: {},
	RecordAccessed: {}, ReferralCreated: {},
}

// Transaction is a signed audit event containing references and a hash only.
// PayloadHash must represent off-chain data; actual clinical data must not be stored here.
type Transaction struct {
	ID             string
	Type           TransactionType
	ActorID        string
	OrganizationID string
	ResourceID     string
	PayloadHash    []byte
	Timestamp      time.Time
	PublicKey      []byte
	Signature      []byte
}

// Hash returns the deterministic SHA-256 hash of the transaction's signable fields.
func (tx Transaction) Hash() [sha256.Size]byte {
	h := sha256.New()
	writeString(h, tx.ID)
	writeString(h, string(tx.Type))
	writeString(h, tx.ActorID)
	writeString(h, tx.OrganizationID)
	writeString(h, tx.ResourceID)
	writeBytes(h, tx.PayloadHash)
	writeUint64(h, uint64(tx.Timestamp.UTC().UnixNano()))
	writeBytes(h, tx.PublicKey)
	var result [sha256.Size]byte
	copy(result[:], h.Sum(nil))
	return result
}

// Sign attaches the public key and an Ed25519 signature to the transaction.
func (tx *Transaction) Sign(privateKey ed25519.PrivateKey) error {
	if len(privateKey) != ed25519.PrivateKeySize {
		return errors.New("invalid Ed25519 private key")
	}
	publicKey := privateKey.Public().(ed25519.PublicKey)
	tx.PublicKey = append([]byte(nil), publicKey...)
	hash := tx.Hash()
	tx.Signature = trustcrypto.Sign(privateKey, hash[:])
	return nil
}

// VerifySignature validates the transaction's Ed25519 signature.
func (tx Transaction) VerifySignature() bool {
	if len(tx.PublicKey) != ed25519.PublicKeySize || len(tx.Signature) != ed25519.SignatureSize {
		return false
	}
	hash := tx.Hash()
	return trustcrypto.Verify(ed25519.PublicKey(tx.PublicKey), hash[:], tx.Signature)
}

func (tx Transaction) validateStructure(position int) error {
	if tx.ID == "" || tx.ActorID == "" || tx.OrganizationID == "" || tx.ResourceID == "" {
		return fmt.Errorf("transaction %d: required identifier is empty", position)
	}
	if _, ok := validTransactionTypes[tx.Type]; !ok {
		return fmt.Errorf("transaction %d: unsupported type %q", position, tx.Type)
	}
	if len(tx.PayloadHash) != sha256.Size {
		return fmt.Errorf("transaction %d: payload hash must be %d bytes", position, sha256.Size)
	}
	if tx.Timestamp.IsZero() {
		return fmt.Errorf("transaction %d: timestamp is required", position)
	}
	return nil
}
