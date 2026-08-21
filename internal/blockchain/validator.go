package blockchain

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
)

var (
	ErrActorInactive         = errors.New("actor inactive")
	ErrOrganizationMismatch  = errors.New("organization mismatch")
	ErrOrganizationInactive  = errors.New("organization inactive")
	ErrPublicKeyMismatch     = errors.New("public key mismatch")
	ErrInvalidSignature      = errors.New("invalid signature")
	ErrValidationUnavailable = errors.New("identity validation unavailable")
)

func (chain *Blockchain) validateTransaction(tx Transaction, position int) error {
	if err := tx.validateStructure(position); err != nil {
		return err
	}
	if chain.registry == nil || chain.policy == nil {
		return fmt.Errorf("transaction %d: %w", position, ErrValidationUnavailable)
	}
	actor, err := chain.registry.GetActor(tx.ActorID)
	if err != nil {
		return fmt.Errorf("transaction %d: %w", position, err)
	}
	organization, err := chain.registry.GetOrganization(tx.OrganizationID)
	if err != nil {
		return fmt.Errorf("transaction %d: %w", position, err)
	}
	if actor.OrganizationID != tx.OrganizationID {
		return fmt.Errorf("transaction %d: %w: actor belongs to %s", position, ErrOrganizationMismatch, actor.OrganizationID)
	}
	if actor.Status != identity.ActorActive {
		return fmt.Errorf("transaction %d: %w: status %s", position, ErrActorInactive, actor.Status)
	}
	if organization.Status != identity.OrganizationActive {
		return fmt.Errorf("transaction %d: %w: status %s", position, ErrOrganizationInactive, organization.Status)
	}
	if !bytes.Equal(actor.PublicKey, tx.PublicKey) {
		return fmt.Errorf("transaction %d: %w", position, ErrPublicKeyMismatch)
	}
	if !tx.VerifySignature() {
		return fmt.Errorf("transaction %d: %w", position, ErrInvalidSignature)
	}
	if err := chain.policy.CanSubmit(actor, string(tx.Type)); err != nil {
		return fmt.Errorf("transaction %d: %w", position, err)
	}
	return nil
}

// Validate checks every block hash, link, height, and transaction signature.
func (chain *Blockchain) Validate() error {
	chain.mu.RLock()
	defer chain.mu.RUnlock()
	if chain == nil || len(chain.blocks) == 0 {
		return errors.New("blockchain is empty")
	}
	for i, block := range chain.blocks {
		if block.Height != uint64(i) {
			return fmt.Errorf("block %d: invalid height %d", i, block.Height)
		}
		if block.Hash != block.CalculateHash() {
			return fmt.Errorf("block %d: hash mismatch", i)
		}
		if i == 0 {
			if block.PreviousHash != ([32]byte{}) {
				return errors.New("genesis block: previous hash must be zero")
			}
		} else if block.PreviousHash != chain.blocks[i-1].Hash {
			return fmt.Errorf("block %d: previous hash mismatch", i)
		}
		for j, tx := range block.Transactions {
			if err := chain.validateTransaction(tx, j); err != nil {
				return fmt.Errorf("block %d: %w", i, err)
			}
		}
	}
	return nil
}
