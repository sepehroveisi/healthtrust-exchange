package besu

import (
	"context"
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

var (
	authorityAssertionRecordedTopic = crypto.Keccak256Hash([]byte("AuthorityAssertionRecorded(bytes32,bytes32,bytes32,bytes32,uint8,uint8,uint8,bytes32,bytes32,int64,uint64)"))
	responseVersionRecordedTopic    = crypto.Keccak256Hash([]byte("ResponseVersionRecorded(bytes32,bytes32,bytes32,bytes32,uint8,bytes32,int64,bytes32,bytes32,bytes32,uint64)"))
)

func (a *Adapter) identityReceipt(ctx context.Context, address common.Address, signature common.Hash, id ledger.ID) (ledger.ChainRecordStatus, ledger.Receipt, error) {
	logs, err := a.client.FilterLogs(ctx, ethereum.FilterQuery{FromBlock: big.NewInt(0), Addresses: []common.Address{address}, Topics: [][]common.Hash{{signature}, {common.BytesToHash(id[:])}}})
	if err != nil {
		return ledger.ChainRecordAbsent, ledger.Receipt{}, fmt.Errorf("%w: identity event lookup", ledger.ErrTransport)
	}
	if len(logs) == 0 {
		return ledger.ChainRecordAbsent, ledger.Receipt{}, nil
	}
	if len(logs) != 1 || logs[0].Removed {
		return ledger.ChainRecordAbsent, ledger.Receipt{}, fmt.Errorf("%w: non-unique identity event", ledger.ErrChainStateAmbiguous)
	}
	var hash ledger.TransactionHash
	copy(hash[:], logs[0].TxHash[:])
	receipt, err := a.ObserveReceipt(ctx, hash)
	if err != nil {
		return ledger.ChainRecordAbsent, ledger.Receipt{}, err
	}
	if receipt.Status == ledger.ReceiptUnknown || receipt.Status == ledger.ReceiptReverted || receipt.BlockNumber != ledger.BlockNumber(logs[0].BlockNumber) || common.BytesToHash(receipt.BlockHash[:]) != logs[0].BlockHash {
		return ledger.ChainRecordAbsent, ledger.Receipt{}, fmt.Errorf("%w: identity event receipt mismatch", ledger.ErrChainStateAmbiguous)
	}
	return ledger.ChainRecordPresent, receipt, nil
}

func (a *Adapter) LookupAuthorityAssertion(ctx context.Context, id ledger.ID) (ledger.AuthorityAssertionLookup, error) {
	status, receipt, err := a.identityReceipt(ctx, a.addresses.AuthorityEventRegistry, authorityAssertionRecordedTopic, id)
	if err != nil || status == ledger.ChainRecordAbsent {
		return ledger.AuthorityAssertionLookup{Status: status}, err
	}
	value, err := a.GetAuthorityAssertion(ctx, id)
	if err != nil {
		return ledger.AuthorityAssertionLookup{}, fmt.Errorf("%w: assertion event without readable state", ledger.ErrChainStateAmbiguous)
	}
	return ledger.AuthorityAssertionLookup{Status: status, Assertion: value, Receipt: receipt}, nil
}

func (a *Adapter) LookupResponseVersion(ctx context.Context, id ledger.ID) (ledger.ResponseVersionLookup, error) {
	status, receipt, err := a.identityReceipt(ctx, a.addresses.ResponseLedger, responseVersionRecordedTopic, id)
	if err != nil || status == ledger.ChainRecordAbsent {
		return ledger.ResponseVersionLookup{Status: status}, err
	}
	value, err := a.GetResponseVersion(ctx, id)
	if err != nil {
		return ledger.ResponseVersionLookup{}, fmt.Errorf("%w: response event without readable state", ledger.ErrChainStateAmbiguous)
	}
	return ledger.ResponseVersionLookup{Status: status, Response: value, Receipt: receipt}, nil
}
