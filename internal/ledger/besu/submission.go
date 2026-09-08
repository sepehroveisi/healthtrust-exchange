package besu

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

func (a *Adapter) broadcast(ctx context.Context, contract *bind.BoundContract, method string, args ...any) (ledger.TransactionHash, error) {
	opts, err := a.txOpts(ctx)
	if err != nil {
		return ledger.TransactionHash{}, err
	}
	tx, err := contract.Transact(opts, method, args...)
	if err != nil {
		return ledger.TransactionHash{}, classify(err)
	}
	var h ledger.TransactionHash
	copy(h[:], tx.Hash().Bytes())
	return h, nil
}
func (a *Adapter) BroadcastAuthorityAssertion(ctx context.Context, v ledger.AuthorityAssertion) (ledger.TransactionHash, error) {
	c := contractAssertion{EventId: v.EventID, EventSeriesId: v.EventSeriesID, PreviousEventId: v.PreviousEventID, TargetEventId: v.TargetEventID, AuthorityId: v.AuthorityID, EventCommitment: v.EventCommitment, EventType: uint8(v.EventType), AssertionKind: uint8(v.AssertionKind), AuthorityEffect: uint8(v.AuthorityEffect), EffectiveTime: v.EffectiveTime}
	return a.broadcast(ctx, a.events, "recordAssertion", c)
}
func (a *Adapter) BroadcastResponseVersion(ctx context.Context, v ledger.ResponseVersion) (ledger.TransactionHash, error) {
	c := contractResponse{v.ResponseID, v.ResponseVersionID, v.PreviousResponseVersionID, v.EventID, v.OrganizationID, uint8(v.State), v.ReceiptTimestamp, v.PolicyVersionHash, v.DecisionCommitment, v.ActionCommitment, v.RecordedAt}
	return a.broadcast(ctx, a.responses, "recordResponseVersion", c)
}
func (a *Adapter) ObserveReceipt(ctx context.Context, h ledger.TransactionHash) (ledger.Receipt, error) {
	r, err := a.client.TransactionReceipt(ctx, common.BytesToHash(h[:]))
	if errors.Is(err, ethereum.NotFound) {
		return ledger.Receipt{Transaction: ledger.Transaction{Hash: h}, Status: ledger.ReceiptUnknown}, nil
	}
	if err != nil {
		return ledger.Receipt{}, fmt.Errorf("%w: receipt", ledger.ErrTransport)
	}
	return receiptResult(r), nil
}
func receiptResult(r *types.Receipt) ledger.Receipt {
	status := ledger.ReceiptReverted
	if r.Status == types.ReceiptStatusSuccessful {
		status = ledger.ReceiptSuccessful
	}
	return ledger.Receipt{Transaction: transaction(r), Status: status}
}
func (a *Adapter) WaitReceipt(ctx context.Context, h ledger.TransactionHash) (ledger.Receipt, error) {
	deadline, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	ticker := time.NewTicker(a.poll)
	defer ticker.Stop()
	for {
		r, err := a.ObserveReceipt(deadline, h)
		if err != nil {
			return ledger.Receipt{}, err
		}
		if r.Status != ledger.ReceiptUnknown {
			return r, nil
		}
		select {
		case <-deadline.Done():
			return ledger.Receipt{}, fmt.Errorf("%w: transaction %s", ledger.ErrReceiptTimeout, h.String())
		case <-ticker.C:
		}
	}
}
