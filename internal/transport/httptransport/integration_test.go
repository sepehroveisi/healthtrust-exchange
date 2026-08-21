package httptransport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/authorization"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/demodata"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/node"
)

func TestHospitalBIndependentlyAcceptsHospitalABlock(t *testing.T) {
	registryA, keys, _ := demodata.Registry()
	registryB, _, _ := demodata.Registry()
	policy := authorization.HealthcarePolicy{}
	genesis := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	client := NewClient(time.Second)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	b := node.New("b", "hospital-b", blockchain.New(genesis, registryB, policy), registryB, policy, nil, client, logger)
	a := node.New("a", "hospital-a", blockchain.New(genesis, registryA, policy), registryA, policy, nil, client, logger)
	payload := sha256.Sum256([]byte("off-chain"))
	tx := blockchain.Transaction{ID: "remote-1", Type: blockchain.RecordCommitted, ActorID: "doctor-a", OrganizationID: "hospital-a", ResourceID: "record-1", PayloadHash: payload[:], Timestamp: genesis.Add(time.Minute)}
	if err := tx.Sign(keys["doctor-a"]); err != nil {
		t.Fatal(err)
	}
	if err := a.SubmitTransaction(tx); err != nil {
		t.Fatal(err)
	}
	block, err := a.CreateBlock(context.Background(), genesis.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	body, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/blocks", bytes.NewReader(body))
	response := httptest.NewRecorder()
	Handler(b).ServeHTTP(response, request)
	if response.Code != http.StatusCreated {
		t.Fatalf("POST /blocks status = %d, body = %s", response.Code, response.Body.String())
	}
	latestA, _ := a.Chain.GetLatestBlock()
	latestB, _ := b.Chain.GetLatestBlock()
	if latestA.Hash != latestB.Hash || latestB.Height != 1 {
		t.Fatal("HTTP-propagated chains did not converge")
	}
}

func TestWorkflowCORSAllowsDraftUpdates(t *testing.T) {
	request := httptest.NewRequest(http.MethodOptions, "/visits/visit-1/draft", nil)
	response := httptest.NewRecorder()
	HandlerWithWorkflow(http.NotFoundHandler(), nil).ServeHTTP(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("OPTIONS status = %d", response.Code)
	}
	if methods := response.Header().Get("Access-Control-Allow-Methods"); methods != "GET, POST, PUT, OPTIONS" {
		t.Fatalf("allowed methods = %q", methods)
	}
}
