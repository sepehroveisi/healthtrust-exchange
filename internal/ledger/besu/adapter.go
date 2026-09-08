package besu

import (
	"context"
	"crypto/ecdsa"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum"
	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

type ContractAddresses struct{ OrganizationRegistry, AuthorityRegistry, AuthorityEventRegistry, ResponseLedger common.Address }
type Config struct {
	RPCURL                       string
	ExpectedChainID              *big.Int
	Contracts                    ContractAddresses
	PrivateKeyHex                string
	ReceiptTimeout, PollInterval time.Duration
}

type Adapter struct {
	client                                     *ethclient.Client
	chainID                                    *big.Int
	signer                                     *ecdsa.PrivateKey
	addresses                                  ContractAddresses
	organization, authority, events, responses *bind.BoundContract
	timeout, poll                              time.Duration
}

const organizationABI = `[{"inputs":[{"name":"organizationId","type":"bytes32"},{"name":"organizationType","type":"uint8"},{"name":"authorizedSubmitter","type":"address"}],"name":"registerOrganization","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"name":"organizationId","type":"bytes32"}],"name":"getOrganization","outputs":[{"components":[{"name":"organizationId","type":"bytes32"},{"name":"organizationType","type":"uint8"},{"name":"active","type":"bool"},{"name":"registeredAt","type":"uint64"},{"name":"authorizedSubmitter","type":"address"}],"name":"","type":"tuple"}],"stateMutability":"view","type":"function"}]`
const authorityABI = `[{"inputs":[{"name":"authorityId","type":"bytes32"},{"name":"authorityType","type":"uint8"},{"name":"authorizedSubmitter","type":"address"}],"name":"registerAuthority","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"name":"authorityId","type":"bytes32"}],"name":"getAuthority","outputs":[{"components":[{"name":"authorityId","type":"bytes32"},{"name":"authorityType","type":"uint8"},{"name":"active","type":"bool"},{"name":"registeredAt","type":"uint64"},{"name":"authorizedSubmitter","type":"address"}],"name":"","type":"tuple"}],"stateMutability":"view","type":"function"}]`
const eventABI = `[{"inputs":[{"components":[{"name":"eventId","type":"bytes32"},{"name":"eventSeriesId","type":"bytes32"},{"name":"previousEventId","type":"bytes32"},{"name":"targetEventId","type":"bytes32"},{"name":"authorityId","type":"bytes32"},{"name":"eventCommitment","type":"bytes32"},{"name":"eventType","type":"uint8"},{"name":"assertionKind","type":"uint8"},{"name":"authorityEffect","type":"uint8"},{"name":"effectiveTime","type":"int64"},{"name":"recordedAt","type":"uint64"}],"name":"assertion","type":"tuple"}],"name":"recordAssertion","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"name":"eventId","type":"bytes32"}],"name":"getAssertion","outputs":[{"components":[{"name":"eventId","type":"bytes32"},{"name":"eventSeriesId","type":"bytes32"},{"name":"previousEventId","type":"bytes32"},{"name":"targetEventId","type":"bytes32"},{"name":"authorityId","type":"bytes32"},{"name":"eventCommitment","type":"bytes32"},{"name":"eventType","type":"uint8"},{"name":"assertionKind","type":"uint8"},{"name":"authorityEffect","type":"uint8"},{"name":"effectiveTime","type":"int64"},{"name":"recordedAt","type":"uint64"}],"name":"","type":"tuple"}],"stateMutability":"view","type":"function"},{"inputs":[{"name":"eventSeriesId","type":"bytes32"}],"name":"currentHead","outputs":[{"name":"","type":"bytes32"}],"stateMutability":"view","type":"function"}]`
const responseABI = `[{"inputs":[{"components":[{"name":"responseId","type":"bytes32"},{"name":"responseVersionId","type":"bytes32"},{"name":"previousResponseVersionId","type":"bytes32"},{"name":"eventId","type":"bytes32"},{"name":"organizationId","type":"bytes32"},{"name":"responseState","type":"uint8"},{"name":"receiptTimestamp","type":"int64"},{"name":"policyVersionHash","type":"bytes32"},{"name":"decisionCommitment","type":"bytes32"},{"name":"actionCommitment","type":"bytes32"},{"name":"recordedAt","type":"uint64"}],"name":"version","type":"tuple"}],"name":"recordResponseVersion","outputs":[],"stateMutability":"nonpayable","type":"function"},{"inputs":[{"name":"responseVersionId","type":"bytes32"}],"name":"getResponseVersion","outputs":[{"components":[{"name":"responseId","type":"bytes32"},{"name":"responseVersionId","type":"bytes32"},{"name":"previousResponseVersionId","type":"bytes32"},{"name":"eventId","type":"bytes32"},{"name":"organizationId","type":"bytes32"},{"name":"responseState","type":"uint8"},{"name":"receiptTimestamp","type":"int64"},{"name":"policyVersionHash","type":"bytes32"},{"name":"decisionCommitment","type":"bytes32"},{"name":"actionCommitment","type":"bytes32"},{"name":"recordedAt","type":"uint64"}],"name":"","type":"tuple"}],"stateMutability":"view","type":"function"},{"inputs":[{"name":"eventId","type":"bytes32"},{"name":"organizationId","type":"bytes32"}],"name":"latestResponseVersionId","outputs":[{"name":"","type":"bytes32"}],"stateMutability":"view","type":"function"}]`

type contractOrganization struct {
	OrganizationId      [32]byte
	OrganizationType    uint8
	Active              bool
	RegisteredAt        uint64
	AuthorizedSubmitter common.Address
}
type contractAuthority struct {
	AuthorityId         [32]byte
	AuthorityType       uint8
	Active              bool
	RegisteredAt        uint64
	AuthorizedSubmitter common.Address
}
type contractAssertion struct {
	EventId, EventSeriesId, PreviousEventId, TargetEventId, AuthorityId, EventCommitment [32]byte
	EventType, AssertionKind, AuthorityEffect                                            uint8
	EffectiveTime                                                                        int64
	RecordedAt                                                                           uint64
}
type contractResponse struct {
	ResponseId, ResponseVersionId, PreviousResponseVersionId, EventId, OrganizationId [32]byte
	ResponseState                                                                     uint8
	ReceiptTimestamp                                                                  int64
	PolicyVersionHash, DecisionCommitment, ActionCommitment                           [32]byte
	RecordedAt                                                                        uint64
}

func New(ctx context.Context, cfg Config) (*Adapter, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}
	client, err := ethclient.DialContext(ctx, cfg.RPCURL)
	if err != nil {
		return nil, fmt.Errorf("%w: connect", ledger.ErrTransport)
	}
	actual, err := client.ChainID(ctx)
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("%w: chain ID", ledger.ErrTransport)
	}
	if actual.Cmp(cfg.ExpectedChainID) != 0 {
		client.Close()
		return nil, fmt.Errorf("%w: expected %s, got %s", ledger.ErrChainIDMismatch, cfg.ExpectedChainID, actual)
	}
	key, err := crypto.HexToECDSA(strings.TrimPrefix(cfg.PrivateKeyHex, "0x"))
	if err != nil {
		client.Close()
		return nil, fmt.Errorf("%w: invalid private key", ledger.ErrSigning)
	}
	parsed := make([]abi.ABI, 4)
	for i, raw := range []string{organizationABI, authorityABI, eventABI, responseABI} {
		parsed[i], err = abi.JSON(strings.NewReader(raw))
		if err != nil {
			client.Close()
			return nil, fmt.Errorf("%w: ABI %d", ledger.ErrInvalidConfiguration, i)
		}
	}
	timeout := cfg.ReceiptTimeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	poll := cfg.PollInterval
	if poll == 0 {
		poll = 200 * time.Millisecond
	}
	return &Adapter{client: client, chainID: new(big.Int).Set(actual), signer: key, addresses: cfg.Contracts, organization: bind.NewBoundContract(cfg.Contracts.OrganizationRegistry, parsed[0], client, client, client), authority: bind.NewBoundContract(cfg.Contracts.AuthorityRegistry, parsed[1], client, client, client), events: bind.NewBoundContract(cfg.Contracts.AuthorityEventRegistry, parsed[2], client, client, client), responses: bind.NewBoundContract(cfg.Contracts.ResponseLedger, parsed[3], client, client, client), timeout: timeout, poll: poll}, nil
}

func validateConfig(cfg Config) error {
	u, err := url.Parse(cfg.RPCURL)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https" && u.Scheme != "ws" && u.Scheme != "wss") {
		return fmt.Errorf("%w: RPC URL", ledger.ErrInvalidConfiguration)
	}
	if cfg.ExpectedChainID == nil || cfg.ExpectedChainID.Sign() <= 0 {
		return fmt.Errorf("%w: expected chain ID", ledger.ErrInvalidConfiguration)
	}
	for _, a := range []common.Address{cfg.Contracts.OrganizationRegistry, cfg.Contracts.AuthorityRegistry, cfg.Contracts.AuthorityEventRegistry, cfg.Contracts.ResponseLedger} {
		if a == (common.Address{}) {
			return fmt.Errorf("%w: zero contract address", ledger.ErrInvalidConfiguration)
		}
	}
	if strings.TrimPrefix(cfg.PrivateKeyHex, "0x") == "" {
		return fmt.Errorf("%w: signer", ledger.ErrInvalidConfiguration)
	}
	if cfg.ReceiptTimeout < 0 || cfg.PollInterval < 0 {
		return fmt.Errorf("%w: timing", ledger.ErrInvalidConfiguration)
	}
	return nil
}

func (a *Adapter) Close()                                    { a.client.Close() }
func (a *Adapter) ChainID(context.Context) (*big.Int, error) { return new(big.Int).Set(a.chainID), nil }
func (a *Adapter) txOpts(ctx context.Context) (*bind.TransactOpts, error) {
	opts, err := bind.NewKeyedTransactorWithChainID(a.signer, a.chainID)
	if err != nil {
		return nil, fmt.Errorf("%w", ledger.ErrSigning)
	}
	opts.Context = ctx
	return opts, nil
}
func classify(err error) error {
	if err == nil {
		return nil
	}
	s := strings.ToLower(err.Error())
	if strings.Contains(s, "revert") {
		return fmt.Errorf("%w: %v", ledger.ErrContractRevert, err)
	}
	return fmt.Errorf("%w: %v", ledger.ErrSubmission, err)
}
func (a *Adapter) submit(ctx context.Context, contract *bind.BoundContract, method string, args ...interface{}) (ledger.Transaction, error) {
	opts, err := a.txOpts(ctx)
	if err != nil {
		return ledger.Transaction{}, err
	}
	tx, err := contract.Transact(opts, method, args...)
	if err != nil {
		return ledger.Transaction{}, classify(err)
	}
	return a.wait(ctx, tx.Hash())
}
func (a *Adapter) wait(ctx context.Context, hash common.Hash) (ledger.Transaction, error) {
	deadlineCtx, cancel := context.WithTimeout(ctx, a.timeout)
	defer cancel()
	ticker := time.NewTicker(a.poll)
	defer ticker.Stop()
	for {
		receipt, err := a.client.TransactionReceipt(deadlineCtx, hash)
		if err == nil {
			if receipt.Status != types.ReceiptStatusSuccessful {
				return ledger.Transaction{}, fmt.Errorf("%w: transaction %s", ledger.ErrContractRevert, hash.Hex())
			}
			return transaction(receipt), nil
		}
		if deadlineCtx.Err() != nil {
			return ledger.Transaction{}, fmt.Errorf("%w: transaction %s", ledger.ErrReceiptTimeout, hash.Hex())
		}
		if !errors.Is(err, ethereum.NotFound) {
			return ledger.Transaction{}, fmt.Errorf("%w: receipt", ledger.ErrTransport)
		}
		select {
		case <-deadlineCtx.Done():
			return ledger.Transaction{}, fmt.Errorf("%w: transaction %s", ledger.ErrReceiptTimeout, hash.Hex())
		case <-ticker.C:
		}
	}
}

func transaction(r *types.Receipt) ledger.Transaction {
	var h, b ledger.TransactionHash
	copy(h[:], r.TxHash[:])
	copy(b[:], r.BlockHash[:])
	return ledger.Transaction{Hash: h, BlockNumber: ledger.BlockNumber(r.BlockNumber.Uint64()), BlockHash: b, ConfirmedAt: time.Now().UTC()}
}
func toAddress(v ledger.Address) common.Address       { return common.BytesToAddress(v[:]) }
func fromAddress(v common.Address) (a ledger.Address) { copy(a[:], v[:]); return }

func (a *Adapter) RegisterOrganization(ctx context.Context, id ledger.ID, t ledger.OrganizationType, s ledger.Address) (ledger.Transaction, error) {
	return a.submit(ctx, a.organization, "registerOrganization", id, uint8(t), toAddress(s))
}
func (a *Adapter) RegisterAuthority(ctx context.Context, id ledger.ID, t ledger.AuthorityType, s ledger.Address) (ledger.Transaction, error) {
	return a.submit(ctx, a.authority, "registerAuthority", id, uint8(t), toAddress(s))
}
func (a *Adapter) RecordAuthorityAssertion(ctx context.Context, v ledger.AuthorityAssertion) (ledger.Transaction, error) {
	c := contractAssertion{EventId: v.EventID, EventSeriesId: v.EventSeriesID, PreviousEventId: v.PreviousEventID, TargetEventId: v.TargetEventID, AuthorityId: v.AuthorityID, EventCommitment: v.EventCommitment, EventType: uint8(v.EventType), AssertionKind: uint8(v.AssertionKind), AuthorityEffect: uint8(v.AuthorityEffect), EffectiveTime: v.EffectiveTime}
	return a.submit(ctx, a.events, "recordAssertion", c)
}
func (a *Adapter) AppendResponseVersion(ctx context.Context, v ledger.ResponseVersion) (ledger.Transaction, error) {
	c := contractResponse{v.ResponseID, v.ResponseVersionID, v.PreviousResponseVersionID, v.EventID, v.OrganizationID, uint8(v.State), v.ReceiptTimestamp, v.PolicyVersionHash, v.DecisionCommitment, v.ActionCommitment, v.RecordedAt}
	return a.submit(ctx, a.responses, "recordResponseVersion", c)
}

func callTuple[T any](ctx context.Context, c *bind.BoundContract, method string, args ...interface{}) (T, error) {
	var zero T
	var out []interface{}
	if err := c.Call(&bind.CallOpts{Context: ctx}, &out, method, args...); err != nil {
		return zero, fmt.Errorf("%w: %v", ledger.ErrTransport, err)
	}
	if len(out) != 1 {
		return zero, fmt.Errorf("%w: unexpected %s output", ledger.ErrTransport, method)
	}
	value, ok := abi.ConvertType(out[0], new(T)).(*T)
	if !ok {
		return zero, fmt.Errorf("%w: decode %s", ledger.ErrTransport, method)
	}
	return *value, nil
}
func (a *Adapter) GetOrganization(ctx context.Context, id ledger.ID) (ledger.Organization, error) {
	v, err := callTuple[contractOrganization](ctx, a.organization, "getOrganization", id)
	if err != nil {
		return ledger.Organization{}, err
	}
	return ledger.Organization{ID: v.OrganizationId, Type: ledger.OrganizationType(v.OrganizationType), Active: v.Active, RegisteredAt: v.RegisteredAt, AuthorizedSubmitter: fromAddress(v.AuthorizedSubmitter)}, nil
}
func (a *Adapter) GetAuthority(ctx context.Context, id ledger.ID) (ledger.Authority, error) {
	v, err := callTuple[contractAuthority](ctx, a.authority, "getAuthority", id)
	if err != nil {
		return ledger.Authority{}, err
	}
	return ledger.Authority{ID: v.AuthorityId, Type: ledger.AuthorityType(v.AuthorityType), Active: v.Active, RegisteredAt: v.RegisteredAt, AuthorizedSubmitter: fromAddress(v.AuthorizedSubmitter)}, nil
}
func (a *Adapter) GetAuthorityAssertion(ctx context.Context, id ledger.ID) (ledger.AuthorityAssertion, error) {
	v, err := callTuple[contractAssertion](ctx, a.events, "getAssertion", id)
	if err != nil {
		return ledger.AuthorityAssertion{}, err
	}
	return ledger.AuthorityAssertion{EventID: v.EventId, EventSeriesID: v.EventSeriesId, PreviousEventID: v.PreviousEventId, TargetEventID: v.TargetEventId, AuthorityID: v.AuthorityId, EventCommitment: v.EventCommitment, EventType: ledger.EventType(v.EventType), AssertionKind: ledger.AssertionKind(v.AssertionKind), AuthorityEffect: ledger.AuthorityEffect(v.AuthorityEffect), EffectiveTime: v.EffectiveTime, RecordedAt: v.RecordedAt}, nil
}
func (a *Adapter) GetCurrentAuthorityHead(ctx context.Context, series ledger.ID) (ledger.ID, error) {
	var out []interface{}
	if err := a.events.Call(&bind.CallOpts{Context: ctx}, &out, "currentHead", series); err != nil {
		return ledger.ID{}, fmt.Errorf("%w: %v", ledger.ErrTransport, err)
	}
	if len(out) != 1 {
		return ledger.ID{}, fmt.Errorf("%w: currentHead output", ledger.ErrTransport)
	}
	return *abi.ConvertType(out[0], new([32]byte)).(*[32]byte), nil
}
func (a *Adapter) GetResponseVersion(ctx context.Context, id ledger.ID) (ledger.ResponseVersion, error) {
	v, err := callTuple[contractResponse](ctx, a.responses, "getResponseVersion", id)
	if err != nil {
		return ledger.ResponseVersion{}, err
	}
	return fromContractResponse(v), nil
}
func (a *Adapter) GetLatestResponse(ctx context.Context, event, org ledger.ID) (ledger.ResponseVersion, error) {
	var out []interface{}
	if err := a.responses.Call(&bind.CallOpts{Context: ctx}, &out, "latestResponseVersionId", event, org); err != nil {
		return ledger.ResponseVersion{}, fmt.Errorf("%w: %v", ledger.ErrTransport, err)
	}
	id := *abi.ConvertType(out[0], new([32]byte)).(*[32]byte)
	return a.GetResponseVersion(ctx, id)
}
func fromContractResponse(v contractResponse) ledger.ResponseVersion {
	return ledger.ResponseVersion{ResponseID: v.ResponseId, ResponseVersionID: v.ResponseVersionId, PreviousResponseVersionID: v.PreviousResponseVersionId, EventID: v.EventId, OrganizationID: v.OrganizationId, State: ledger.ResponseState(v.ResponseState), ReceiptTimestamp: v.ReceiptTimestamp, PolicyVersionHash: v.PolicyVersionHash, DecisionCommitment: v.DecisionCommitment, ActionCommitment: v.ActionCommitment, RecordedAt: v.RecordedAt}
}
func (a *Adapter) TransactionReceipt(ctx context.Context, h ledger.TransactionHash) (ledger.Receipt, error) {
	r, err := a.client.TransactionReceipt(ctx, common.BytesToHash(h[:]))
	if err != nil {
		return ledger.Receipt{}, fmt.Errorf("%w: receipt", ledger.ErrTransport)
	}
	status := ledger.ReceiptReverted
	if r.Status == types.ReceiptStatusSuccessful {
		status = ledger.ReceiptSuccessful
	}
	return ledger.Receipt{Transaction: transaction(r), Status: status}, nil
}
