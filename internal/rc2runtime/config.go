package rc2runtime

import (
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
)

const bootstrapEnvironment = "HEALTHTRUST_RC2_DEMO_BOOTSTRAP_ENABLED"

type Config struct {
	ListenAddress      string
	DatabaseURL        string
	MigrationDirectory string
	RPCURL             string
	PeerRPCURL         string
	ChainID            *big.Int
	Contracts          besu.ContractAddresses
	AuthorityKey       string
	HospitalKey        string
	PayerKey           string
	StaffingKey        string
	AllowedOrigin      string
	BootstrapEnabled   bool
	ReceiptTimeout     time.Duration
	PollInterval       time.Duration
}

func ConfigFromEnvironment() (Config, error) {
	cfg := Config{
		ListenAddress:      value("HEALTHTRUST_RC2_LISTEN_ADDRESS", ":8090"),
		DatabaseURL:        os.Getenv("HEALTHTRUST_RC2_DATABASE_URL"),
		MigrationDirectory: value("HEALTHTRUST_RC2_MIGRATIONS", "migrations/authority_ledger"),
		RPCURL:             os.Getenv("HEALTHTRUST_RC2_BESU_RPC_URL"),
		PeerRPCURL:         os.Getenv("HEALTHTRUST_RC2_BESU_PEER_RPC_URL"),
		AuthorityKey:       os.Getenv("DEMO_AUTHORITY_PRIVATE_KEY"),
		HospitalKey:        os.Getenv("DEMO_HOSPITAL_PRIVATE_KEY"),
		PayerKey:           os.Getenv("DEMO_PAYER_PRIVATE_KEY"),
		StaffingKey:        os.Getenv("DEMO_STAFFING_PRIVATE_KEY"),
		AllowedOrigin:      os.Getenv("HEALTHTRUST_RC2_ALLOWED_ORIGIN"),
		BootstrapEnabled:   os.Getenv(bootstrapEnvironment) == "true",
		ReceiptTimeout:     30 * time.Second,
		PollInterval:       200 * time.Millisecond,
	}
	chainID, ok := new(big.Int).SetString(os.Getenv("HEALTHTRUST_RC2_CHAIN_ID"), 10)
	if !ok {
		return Config{}, errors.New("HEALTHTRUST_RC2_CHAIN_ID must be a positive base-10 integer")
	}
	cfg.ChainID = chainID
	addresses := []struct {
		name string
		dst  *common.Address
	}{
		{"HEALTHTRUST_RC2_ORGANIZATION_REGISTRY_ADDRESS", &cfg.Contracts.OrganizationRegistry},
		{"HEALTHTRUST_RC2_AUTHORITY_REGISTRY_ADDRESS", &cfg.Contracts.AuthorityRegistry},
		{"HEALTHTRUST_RC2_AUTHORITY_EVENT_REGISTRY_ADDRESS", &cfg.Contracts.AuthorityEventRegistry},
		{"HEALTHTRUST_RC2_RESPONSE_LEDGER_ADDRESS", &cfg.Contracts.ResponseLedger},
	}
	for _, item := range addresses {
		raw := os.Getenv(item.name)
		if !common.IsHexAddress(raw) || common.HexToAddress(raw) == (common.Address{}) {
			return Config{}, fmt.Errorf("%s must be a non-zero Ethereum address", item.name)
		}
		*item.dst = common.HexToAddress(raw)
	}
	if raw := os.Getenv("HEALTHTRUST_RC2_RECEIPT_TIMEOUT_SECONDS"); raw != "" {
		seconds, err := strconv.Atoi(raw)
		if err != nil || seconds <= 0 {
			return Config{}, errors.New("HEALTHTRUST_RC2_RECEIPT_TIMEOUT_SECONDS must be positive")
		}
		cfg.ReceiptTimeout = time.Duration(seconds) * time.Second
	}
	return cfg, cfg.Validate()
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.ListenAddress) == "" || strings.TrimSpace(c.DatabaseURL) == "" || strings.TrimSpace(c.MigrationDirectory) == "" {
		return errors.New("listen address, PostgreSQL URL, and migration directory are required")
	}
	if err := validRPC(c.RPCURL); err != nil {
		return fmt.Errorf("primary Besu RPC: %w", err)
	}
	if err := validRPC(c.PeerRPCURL); err != nil {
		return fmt.Errorf("peer Besu RPC: %w", err)
	}
	if c.ChainID == nil || c.ChainID.Sign() <= 0 {
		return errors.New("chain ID must be positive")
	}
	for _, address := range []common.Address{c.Contracts.OrganizationRegistry, c.Contracts.AuthorityRegistry, c.Contracts.AuthorityEventRegistry, c.Contracts.ResponseLedger} {
		if address == (common.Address{}) {
			return errors.New("all contract addresses are required")
		}
	}
	for _, key := range []string{c.AuthorityKey, c.HospitalKey, c.PayerKey, c.StaffingKey} {
		if strings.TrimPrefix(key, "0x") == "" {
			return errors.New("all four application signer keys are required")
		}
	}
	if c.AllowedOrigin != "" {
		u, err := url.Parse(c.AllowedOrigin)
		if err != nil || u.Scheme == "" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return errors.New("allowed origin must be an exact origin without path, query, or fragment")
		}
	}
	return nil
}

func validRPC(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("URL must be HTTP(S)")
	}
	return nil
}

func value(name, fallback string) string {
	if current := os.Getenv(name); current != "" {
		return current
	}
	return fallback
}
