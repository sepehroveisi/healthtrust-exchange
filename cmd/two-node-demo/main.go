package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"net/http/httptest"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	trustcrypto "github.com/healthtrust-exchange/healthtrust-exchange/internal/cryptography"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/transport/httptransport"
)

func main() {
	registryA, keysA, _ := demodata.Registry()
	registryB, _, _ := demodata.Registry()
	policy := authorization.HealthcarePolicy{}
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	client := httptransport.NewClient(3 * time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	nodeB := node.New("hospital-b-node", "hospital-b", blockchain.New(genesis, registryB, policy), registryB, policy, nil, client, logger)
	serverB := httptest.NewServer(httptransport.Handler(nodeB))
	defer serverB.Close()
	nodeA := node.New("hospital-a-node", "hospital-a", blockchain.New(genesis, registryA, policy), registryA, policy, []node.Peer{{ID: "hospital-b-node", OrganizationID: "hospital-b", BaseURL: serverB.URL}}, client, logger)
	payloadHash := sha256.Sum256([]byte("fictional record stored off-chain"))
	now := genesis.Add(time.Hour)
	tx := blockchain.Transaction{ID: "demo-record-1", Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "fictional-record-1", PayloadHash: payloadHash[:], Timestamp: now}
	_ = tx.Sign(keysA["doctor-a"])
	_ = nodeA.SubmitTransaction(tx)
	_, _ = nodeA.CreateBlock(context.Background(), now)
	latestA, _ := nodeA.Chain.GetLatestBlock()
	latestB, _ := nodeB.Chain.GetLatestBlock()
	_, attackerPrivateKey, _ := trustcrypto.GenerateKeyPair()
	forged := tx
	forged.ID = "forged-record"
	_ = forged.Sign(attackerPrivateKey)
	forgedErr := nodeA.SubmitTransaction(forged)
	fmt.Printf("Hospital A height: %d\nHospital B height: %d\nChains equal: %t\nForged transaction rejected: %t\n", latestA.Height, latestB.Height, latestA.Hash == latestB.Hash, forgedErr != nil)
}
