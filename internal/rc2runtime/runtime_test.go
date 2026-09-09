package rc2runtime

import (
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
)

func TestExactOriginCORS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	handler := exactOrigin("http://localhost:3000", next)

	allowed := httptest.NewRequest(http.MethodOptions, "/api/rc2/authority-events", nil)
	allowed.Header.Set("Origin", "http://localhost:3000")
	allowed.Header.Set("Access-Control-Request-Method", "GET")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, allowed)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Fatalf("allowed preflight: status=%d origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}

	denied := httptest.NewRequest(http.MethodOptions, "/api/rc2/authority-events", nil)
	denied.Header.Set("Origin", "https://example.invalid")
	denied.Header.Set("Access-Control-Request-Method", "GET")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, denied)
	if response.Code != http.StatusForbidden || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("denied preflight: status=%d origin=%q", response.Code, response.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestConfigRejectsOriginWithPath(t *testing.T) {
	cfg := validTestConfig()
	cfg.AllowedOrigin = "http://localhost:3000/path"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid exact origin")
	}
}

func validTestConfig() Config {
	return Config{
		ListenAddress:      ":8090",
		DatabaseURL:        "postgres://demo:demo@localhost/demo",
		MigrationDirectory: "testdata/migrations",
		RPCURL:             "http://localhost:8545",
		PeerRPCURL:         "http://localhost:8546",
		ChainID:            big.NewInt(202603),
		Contracts: besu.ContractAddresses{
			OrganizationRegistry:   common.HexToAddress("0x1"),
			AuthorityRegistry:      common.HexToAddress("0x2"),
			AuthorityEventRegistry: common.HexToAddress("0x3"),
			ResponseLedger:         common.HexToAddress("0x4"),
		},
		AuthorityKey:   "01",
		HospitalKey:    "01",
		PayerKey:       "01",
		StaffingKey:    "01",
		AllowedOrigin:  "http://localhost:3000",
		ReceiptTimeout: time.Minute,
		PollInterval:   time.Second,
	}
}
