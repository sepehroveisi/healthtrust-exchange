package main

import (
	"context"
	"crypto/ed25519"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/config"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
	pgstore "github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/postgres"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/recovery"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/transport/httptransport"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/workflow"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := config.FromEnvironment()
	if err != nil {
		logger.Error("configuration_invalid", "error", err)
		os.Exit(1)
	}
	ctx := context.Background()
	store, err := pgstore.Open(ctx, config.DatabaseURL)
	if err != nil {
		logger.Error("database_connection_failed", "error", err)
		os.Exit(1)
	}
	defer store.Close()
	logger.Info("database_connected")
	if err := store.Migrate(ctx); err != nil {
		logger.Error("migration_failed", "error", err)
		os.Exit(1)
	}
	policy := authorization.HealthcarePolicy{}
	genesisTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	state, err := recovery.Load(ctx, store, policy, genesisTime, true)
	if err != nil {
		logger.Error("recovery_validation_failed", "error", err)
		os.Exit(1)
	}
	latest, _ := state.Chain.GetLatestBlock()
	logger.Info("identity_loaded")
	logger.Info("chain_recovered", "height", latest.Height)
	client := httptransport.NewClient(config.RequestTimeout)
	hospital := node.New(config.NodeID, config.OrganizationID, state.Chain, state.Registry, policy, config.Peers, client, logger)
	hospital.SetBlockStore(store)
	_, keys, err := demodata.Registry()
	if err != nil {
		logger.Error("demo_keys_failed", "error", err)
		os.Exit(1)
	}
	trustedPeers := map[string]security.Peer{}
	peerURLs := map[string]string{}
	for _, v := range []struct{ id, org string }{{"hospital-a-node", "hospital-a"}, {"hospital-b-node", "hospital-b"}} {
		if v.id != config.NodeID {
			k := demodata.NodeKey(v.id)
			trustedPeers[v.id] = security.Peer{NodeID: v.id, OrganizationID: v.org, PublicKey: k.Public().(ed25519.PublicKey)}
		}
	}
	for _, peer := range config.Peers {
		targetID := "hospital-a-node"
		if config.NodeID == targetID {
			targetID = "hospital-b-node"
		}
		peerURLs[targetID] = peer.BaseURL
	}
	workflowService := &workflow.Service{Store: store, Node: hospital, Registry: state.Registry, Keys: keys, Logger: logger, NodeID: config.NodeID, OrganizationID: config.OrganizationID, TrustedPeers: trustedPeers, PeerURLs: peerURLs, NodeKey: demodata.NodeKey(config.NodeID), HTTPClient: &http.Client{Timeout: config.RequestTimeout}}
	handler := httptransport.HandlerWithWorkflow(httptransport.Handler(hospital), workflowService)
	server := &http.Server{Addr: config.ListenAddress, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		logger.Info("node_started", "node_id", hospital.ID, "address", config.ListenAddress)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server_failed", "error", err)
			os.Exit(1)
		}
	}()
	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	<-stop.Done()
	shutdownContext, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownContext)
}
