package besu

import (
	"context"
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

func validTestConfig() Config {
	address := common.HexToAddress("0x0000000000000000000000000000000000000001")
	return Config{RPCURL: "http://127.0.0.1:1", ExpectedChainID: big.NewInt(202603), Contracts: ContractAddresses{address, address, address, address}, PrivateKeyHex: "01"}
}

func TestRejectsInvalidConfiguration(t *testing.T) {
	cfg := validTestConfig()
	cfg.RPCURL = "file:///tmp/socket"
	if _, err := New(context.Background(), cfg); !errors.Is(err, ledger.ErrInvalidConfiguration) {
		t.Fatalf("got %v", err)
	}
	cfg = validTestConfig()
	cfg.Contracts.ResponseLedger = common.Address{}
	if _, err := New(context.Background(), cfg); !errors.Is(err, ledger.ErrInvalidConfiguration) {
		t.Fatalf("got %v", err)
	}
}

func TestUnreachableRPCIsTyped(t *testing.T) {
	if _, err := New(context.Background(), validTestConfig()); !errors.Is(err, ledger.ErrTransport) {
		t.Fatalf("got %v", err)
	}
}
