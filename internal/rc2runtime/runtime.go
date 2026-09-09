package rc2runtime

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/ethereum/go-ethereum/crypto"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/tamperdemo"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/transport/rc2http"
)

type Runtime struct {
	Handler http.Handler
	Store   *authorityledger.Store
	closers []*besu.Adapter
}

func New(ctx context.Context, cfg Config) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	store, err := authorityledger.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("open RC2 PostgreSQL: %w", err)
	}
	fail := func(err error, adapters ...*besu.Adapter) (*Runtime, error) {
		for _, adapter := range adapters {
			if adapter != nil {
				adapter.Close()
			}
		}
		store.Close()
		return nil, err
	}
	if err = authorityledger.NewMigrator(store.Pool(), cfg.MigrationDirectory).Up(ctx); err != nil {
		return fail(fmt.Errorf("migrate RC2 PostgreSQL: %w", err))
	}
	keys := []string{cfg.AuthorityKey, cfg.HospitalKey, cfg.PayerKey, cfg.StaffingKey}
	adapters := make([]*besu.Adapter, 0, 5)
	for _, key := range keys {
		adapter, adapterErr := newAdapter(ctx, cfg, cfg.RPCURL, key)
		if adapterErr != nil {
			return fail(adapterErr, adapters...)
		}
		adapters = append(adapters, adapter)
	}
	peer, err := newAdapter(ctx, cfg, cfg.PeerRPCURL, cfg.AuthorityKey)
	if err != nil {
		return fail(err, adapters...)
	}
	adapters = append(adapters, peer)
	if err = validateLedgerConfiguration(ctx, adapters[0], peer, keys); err != nil {
		return fail(err, adapters...)
	}

	signerIDs := []string{FixtureAuthorityID, FixtureHospitalID, FixturePayerID, FixtureStaffingID}
	submitters := make(map[string]ledger.SubmissionInterface, 4)
	reconcilers := make(map[string]ledger.ReconciliationInterface, 4)
	for index, id := range signerIDs {
		submitters[id] = adapters[index]
		reconcilers[id] = adapters[index]
	}
	reconciler := submission.NewReconciler(store, reconcilers)
	workflow := authorityworkflow.New(store, submission.New(store, submitters), reconciler)
	verifier := integrityverification.New(store, adapters[0])
	if cfg.BootstrapEnabled {
		if err = Bootstrap(ctx, store, workflow, verifier, keys); err != nil {
			return fail(fmt.Errorf("bootstrap RC2 demo: %w", err), adapters...)
		}
	}
	demo := tamperdemo.New(store, store.Pool(), verifier)
	api := rc2http.HandlerWithDemo(workflow, verifier, demo)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"status\":\"ok\"}\n"))
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := store.Pool().Ping(r.Context()); err != nil {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("{\"status\":\"ready\"}\n"))
	})
	mux.Handle("/", api)
	return &Runtime{Handler: exactOrigin(cfg.AllowedOrigin, mux), Store: store, closers: adapters}, nil
}

func newAdapter(ctx context.Context, cfg Config, rpc, key string) (*besu.Adapter, error) {
	adapter, err := besu.New(ctx, besu.Config{RPCURL: rpc, ExpectedChainID: cfg.ChainID, Contracts: cfg.Contracts, PrivateKeyHex: key, ReceiptTimeout: cfg.ReceiptTimeout, PollInterval: cfg.PollInterval})
	if err != nil {
		return nil, fmt.Errorf("connect RC2 Besu adapter: %w", err)
	}
	return adapter, nil
}

func validateLedgerConfiguration(ctx context.Context, primary, peer *besu.Adapter, keys []string) error {
	expectedAddresses := make([]ledger.Address, len(keys))
	for index, key := range keys {
		privateKey, err := crypto.HexToECDSA(strings.TrimPrefix(key, "0x"))
		if err != nil {
			return fmt.Errorf("application signer %d: %w", index, err)
		}
		copy(expectedAddresses[index][:], crypto.PubkeyToAddress(privateKey.PublicKey).Bytes())
	}
	for _, reader := range []*besu.Adapter{primary, peer} {
		authority, err := reader.GetAuthority(ctx, ledger.MustID(FixtureAuthorityID))
		if err != nil {
			return fmt.Errorf("read frozen authority contract configuration: %w", err)
		}
		if !authority.Active || authority.Type != ledger.AuthorityTypeExclusion || authority.AuthorizedSubmitter != expectedAddresses[0] {
			return fmt.Errorf("frozen authority contract configuration mismatch for %s", FixtureAuthorityID)
		}
		organizations := []struct {
			id        string
			typeValue ledger.OrganizationType
			address   ledger.Address
		}{
			{FixtureHospitalID, ledger.OrganizationTypeHospital, expectedAddresses[1]},
			{FixturePayerID, ledger.OrganizationTypePayer, expectedAddresses[2]},
			{FixtureStaffingID, ledger.OrganizationTypeStaffingAgency, expectedAddresses[3]},
		}
		for _, expected := range organizations {
			organization, loadErr := reader.GetOrganization(ctx, ledger.MustID(expected.id))
			if loadErr != nil {
				return fmt.Errorf("read frozen organization contract configuration for %s: %w", expected.id, loadErr)
			}
			if !organization.Active || organization.Type != expected.typeValue || organization.AuthorizedSubmitter != expected.address {
				return fmt.Errorf("frozen organization contract configuration mismatch for %s", expected.id)
			}
		}
	}
	return nil
}

func (r *Runtime) Close() {
	for _, closer := range r.closers {
		closer.Close()
	}
	r.Store.Close()
}

func exactOrigin(origin string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestOrigin := r.Header.Get("Origin")
		if origin != "" && requestOrigin == origin {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Organization-ID")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		}
		if r.Method == http.MethodOptions {
			if origin == "" || requestOrigin != origin || strings.TrimSpace(r.Header.Get("Access-Control-Request-Method")) == "" {
				http.Error(w, "origin not allowed", http.StatusForbidden)
				return
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
