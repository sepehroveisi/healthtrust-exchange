//go:build integration

package integrityverification_test

import (
	"context"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	workflow "github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	verify "github.com/healthtrust-exchange/healthtrust-exchange/internal/application/integrityverification"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/transport/rc2http"
)

const (
	authorityKey = "00000000000000000000000000000000000000000000000000000000000a11ce"
	hospitalKey  = "0000000000000000000000000000000000000000000000000000000000000b0b"
	payerKey     = "000000000000000000000000000000000000000000000000000000000000ca11"
	staffingKey  = "0000000000000000000000000000000000000000000000000000000000c0ffee"
)

var contracts = besu.ContractAddresses{OrganizationRegistry: common.HexToAddress("0x1148e16d870cc191b97774d1b55f65d42ecf6cd7"), AuthorityRegistry: common.HexToAddress("0xf5495ad34a7f25b26bb8b8ef512c44c7cb5cc005"), AuthorityEventRegistry: common.HexToAddress("0x3372e3af8edce113af8926a924f823edf2a301c2"), ResponseLedger: common.HexToAddress("0xaebe3a5fc1b9049970d0a53967d07c98211bcde7")}

func adapter(t *testing.T, rpc, key string) *besu.Adapter {
	t.Helper()
	value, err := besu.New(context.Background(), besu.Config{RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: contracts, PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(value.Close)
	return value
}

type readerOverride struct {
	verify.LedgerReader
	unavailableOrg string
	mutateEvent    func(*ledger.AuthorityAssertion)
	mutateResponse func(*ledger.ResponseVersion)
	mutateReceipt  func(*ledger.Receipt)
}

func (r readerOverride) GetAuthorityAssertion(ctx context.Context, id ledger.ID) (ledger.AuthorityAssertion, error) {
	value, err := r.LedgerReader.GetAuthorityAssertion(ctx, id)
	if err == nil && r.mutateEvent != nil {
		r.mutateEvent(&value)
	}
	return value, err
}
func (r readerOverride) GetResponseVersion(ctx context.Context, id ledger.ID) (ledger.ResponseVersion, error) {
	value, err := r.LedgerReader.GetResponseVersion(ctx, id)
	if err == nil && r.unavailableOrg != "" && value.OrganizationID.String() == r.unavailableOrg {
		return ledger.ResponseVersion{}, ledger.ErrTransport
	}
	if err == nil && r.mutateResponse != nil {
		r.mutateResponse(&value)
	}
	return value, err
}
func (r readerOverride) GetLatestResponse(ctx context.Context, event, org ledger.ID) (ledger.ResponseVersion, error) {
	value, err := r.LedgerReader.GetLatestResponse(ctx, event, org)
	if err == nil && r.unavailableOrg != "" && value.OrganizationID.String() == r.unavailableOrg {
		return ledger.ResponseVersion{}, ledger.ErrTransport
	}
	if err == nil && r.mutateResponse != nil {
		r.mutateResponse(&value)
	}
	return value, err
}
func (r readerOverride) TransactionReceipt(ctx context.Context, hash ledger.TransactionHash) (ledger.Receipt, error) {
	value, err := r.LedgerReader.TransactionReceipt(ctx, hash)
	if err == nil && r.mutateReceipt != nil {
		r.mutateReceipt(&value)
	}
	return value, err
}

type unavailableReader struct{}

func (unavailableReader) GetAuthorityAssertion(context.Context, ledger.ID) (ledger.AuthorityAssertion, error) {
	return ledger.AuthorityAssertion{}, ledger.ErrTransport
}
func (unavailableReader) GetCurrentAuthorityHead(context.Context, ledger.ID) (ledger.ID, error) {
	return ledger.ID{}, ledger.ErrTransport
}
func (unavailableReader) GetResponseVersion(context.Context, ledger.ID) (ledger.ResponseVersion, error) {
	return ledger.ResponseVersion{}, ledger.ErrTransport
}
func (unavailableReader) GetLatestResponse(context.Context, ledger.ID, ledger.ID) (ledger.ResponseVersion, error) {
	return ledger.ResponseVersion{}, ledger.ErrTransport
}
func (unavailableReader) TransactionReceipt(context.Context, ledger.TransactionHash) (ledger.Receipt, error) {
	return ledger.Receipt{}, ledger.ErrTransport
}

type durableSnapshot struct{ assertions, responses, submissions, attempts int }

func snapshot(t *testing.T, store *authorityledger.Store) durableSnapshot {
	t.Helper()
	var result durableSnapshot
	ctx := context.Background()
	for table, target := range map[string]*int{"authority_assertions": &result.assertions, "response_versions": &result.responses, "ledger_submissions": &result.submissions} {
		value, err := store.Count(ctx, table)
		if err != nil {
			t.Fatal(err)
		}
		*target = value
	}
	if err := store.Pool().QueryRow(ctx, `SELECT COALESCE(sum(attempt_count),0) FROM authority_ledger.ledger_submissions`).Scan(&result.attempts); err != nil {
		t.Fatal(err)
	}
	return result
}

func requireStatus(t *testing.T, name string, got verify.VerificationStatus, want verify.VerificationStatus) {
	t.Helper()
	if got != want {
		t.Fatalf("%s status=%s want=%s", name, got, want)
	}
}

func TestPhase7IntegrityVerificationPostgresBesu(t *testing.T) {
	databaseURL := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	store, err := authorityledger.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	migrator := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "..", "migrations", "authority_ledger"))
	_ = migrator.DownAll(ctx)
	if err = migrator.Up(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = migrator.DownAll(ctx); store.Close() })

	var address authorityledger.Address
	organizations := []authorityledger.Organization{{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: address}, {ID: "PAYER-B", Type: "PAYER", DisplayName: "Payer B", Active: true, Signer: address}, {ID: "STAFFING-AGENCY-C", Type: "STAFFING_AGENCY", DisplayName: "Staffing Agency C", Active: true, Signer: address}}
	for _, organization := range organizations {
		if err = store.CreateOrganization(ctx, organization); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: address}); err != nil {
		t.Fatal(err)
	}
	if err = store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-7F31A", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); err != nil {
		t.Fatal(err)
	}

	authority := adapter(t, "http://127.0.0.1:8545", authorityKey)
	hospital := adapter(t, "http://127.0.0.1:8545", hospitalKey)
	payer := adapter(t, "http://127.0.0.1:8545", payerKey)
	staffing := adapter(t, "http://127.0.0.1:8545", staffingKey)
	submitters := map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	reconcilers := map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	workflowService := workflow.New(store, submission.New(store, submitters), submission.NewReconciler(store, reconcilers))
	eventTime := time.Date(2026, 6, 1, 0, 0, 0, 123, time.UTC)
	source := []byte("synthetic private authority source")
	event, err := workflowService.RegisterAuthorityEvent(ctx, workflow.AuthorityEventInput{EventID: "EVENT-PHASE7", SeriesID: "SERIES-PHASE7", AuthorityID: "HHS-OIG-DEMO", SubjectID: "PRV-7F31A", OperationID: "OP-P7-EVENT", EffectiveTime: eventTime, SourceDocumentHash: canonical.Hash(source), SourceDocument: source})
	if err != nil || event.Ledger.State != "CONFIRMED" {
		t.Fatalf("event %+v %v", event, err)
	}

	type flow struct{ org, short, response string }
	flows := []flow{{"HOSPITAL-A", "HOSP", "P7-HOSP"}, {"PAYER-B", "PAYER", "P7-PAYER"}, {"STAFFING-AGENCY-C", "STAFF", "P7-STAFF"}}
	for index, flow := range flows {
		receiptTime := eventTime.Add(time.Duration(index+1) * time.Hour)
		if value, startErr := workflowService.StartOrganizationResponse(ctx, workflow.StartResponseInput{ActorOrganizationID: flow.org, OrganizationID: flow.org, EventID: event.ID, ResponseID: flow.response, VersionID: flow.response + "-V1", OperationID: "OP-P7-" + flow.short + "-V1", ReceiptTime: receiptTime}); startErr != nil || value.Ledger.State != "CONFIRMED" {
			t.Fatalf("start %s %+v %v", flow.org, value, startErr)
		}
		for stateIndex, state := range []string{"UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"} {
			version := stateIndex + 2
			input := workflow.AdvanceResponseInput{ActorOrganizationID: flow.org, OrganizationID: flow.org, EventID: event.ID, VersionID: flow.response + "-V" + string(rune('0'+version)), OperationID: "OP-P7-" + flow.short + "-V" + string(rune('0'+version)), TargetState: state, OccurredAt: receiptTime.Add(time.Duration(version) * time.Hour), PolicyID: "POL-P7-" + flow.short, PolicyVersionID: "POL-P7-" + flow.short + "-V1", PolicyText: "private " + flow.short + " policy", DecisionID: "DEC-P7-" + flow.short, DecisionCode: "HOLD-" + flow.short, DecisionExplanation: "private " + flow.short + " decision", ReviewerReference: "REVIEW-" + flow.short, ActionID: "ACT-P7-" + flow.short, ActionCode: "COMPLETE-" + flow.short, ActionExplanation: "private " + flow.short + " action", OperatorReference: "OPERATOR-" + flow.short, SupportingEvidence: []byte("private " + flow.short + " support")}
			if value, advanceErr := workflowService.AdvanceOrganizationResponse(ctx, input); advanceErr != nil || value.Ledger.State != "CONFIRMED" {
				t.Fatalf("advance %s %s %+v %v", flow.org, state, value, advanceErr)
			}
		}
	}

	verifier := verify.New(store, authority)
	clean, err := verifier.VerifyEventBundle(ctx, event.ID)
	if err != nil || len(clean.Responses) != 3 {
		t.Fatalf("clean bundle %+v %v", clean, err)
	}
	requireStatus(t, "authority", clean.Authority.Status, verify.StatusVerified)
	for _, response := range clean.Responses {
		requireStatus(t, response.OrganizationID, response.Result.Status, verify.StatusVerified)
	}
	requireStatus(t, "bundle", clean.Status, verify.StatusVerified)
	if again, repeatErr := verifier.VerifyEventBundle(ctx, event.ID); repeatErr != nil || again.Status != clean.Status {
		t.Fatalf("non-deterministic repeat %+v %v", again, repeatErr)
	}
	before := snapshot(t, store)

	if _, err = store.Pool().Exec(ctx, `UPDATE authority_ledger.authority_assertions SET source_document=$1 WHERE id=$2`, []byte("mutated authority source"), event.ID); err != nil {
		t.Fatal(err)
	}
	mutatedAuthority, err := verifier.VerifyAuthorityEvent(ctx, event.ID)
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, "mutated authority", mutatedAuthority.Status, verify.StatusFailed)
	for _, flow := range flows {
		result, responseErr := verifier.VerifyResponse(ctx, event.ID, flow.org)
		if responseErr != nil {
			t.Fatal(responseErr)
		}
		requireStatus(t, "authority-isolated "+flow.org, result.Status, verify.StatusVerified)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE authority_ledger.authority_assertions SET source_document=$1 WHERE id=$2`, source, event.ID); err != nil {
		t.Fatal(err)
	}

	if _, err = store.Pool().Exec(ctx, `UPDATE authority_ledger.policies SET policy_text=$1 WHERE version_id=$2`, "mutated hospital policy", "POL-P7-HOSP-V1"); err != nil {
		t.Fatal(err)
	}
	hospitalResult, err := verifier.VerifyResponse(ctx, event.ID, "HOSPITAL-A")
	if err != nil {
		t.Fatal(err)
	}
	requireStatus(t, "mutated hospital", hospitalResult.Status, verify.StatusFailed)
	for _, organization := range []string{"PAYER-B", "STAFFING-AGENCY-C"} {
		result, responseErr := verifier.VerifyResponse(ctx, event.ID, organization)
		if responseErr != nil {
			t.Fatal(responseErr)
		}
		requireStatus(t, "isolated "+organization, result.Status, verify.StatusVerified)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE authority_ledger.policies SET policy_text=$1 WHERE version_id=$2`, "private HOSP policy", "POL-P7-HOSP-V1"); err != nil {
		t.Fatal(err)
	}

	peer := adapter(t, "http://127.0.0.1:8546", authorityKey)
	peerBundle, err := verify.New(store, peer).VerifyEventBundle(ctx, event.ID)
	if err != nil || peerBundle.Status != verify.StatusVerified {
		t.Fatalf("second node %+v %v", peerBundle, err)
	}

	unavailableBundle, unavailableErr := verify.New(store, unavailableReader{}).VerifyEventBundle(ctx, event.ID)
	if !errors.Is(unavailableErr, verify.ErrUnavailable) || unavailableBundle.Status != verify.StatusIndeterminate {
		t.Fatalf("unavailable bundle %+v %v", unavailableBundle, unavailableErr)
	}
	partial, partialErr := verify.New(store, readerOverride{LedgerReader: authority, unavailableOrg: "PAYER-B"}).VerifyEventBundle(ctx, event.ID)
	if !errors.Is(partialErr, verify.ErrUnavailable) || partial.Status != verify.StatusIndeterminate {
		t.Fatalf("partial bundle %+v %v", partial, partialErr)
	}
	mixed, mixedErr := verify.New(store, readerOverride{LedgerReader: authority, unavailableOrg: "PAYER-B", mutateEvent: func(v *ledger.AuthorityAssertion) { v.EventCommitment[0]++ }}).VerifyEventBundle(ctx, event.ID)
	if mixedErr != nil || mixed.Status != verify.StatusFailed {
		t.Fatalf("failure precedence %+v %v", mixed, mixedErr)
	}

	mismatchCases := []struct {
		name   string
		reader verify.LedgerReader
		call   func(*verify.Service) (verify.Result, error)
	}{
		{"authority commitment", readerOverride{LedgerReader: authority, mutateEvent: func(v *ledger.AuthorityAssertion) { v.EventCommitment[0]++ }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyAuthorityEvent(ctx, event.ID) }},
		{"response identity", readerOverride{LedgerReader: authority, mutateResponse: func(v *ledger.ResponseVersion) { v.ResponseID = ledger.MustID("OTHER") }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyResponse(ctx, event.ID, "HOSPITAL-A") }},
		{"response state", readerOverride{LedgerReader: authority, mutateResponse: func(v *ledger.ResponseVersion) { v.State = ledger.ResponseReceived }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyResponse(ctx, event.ID, "HOSPITAL-A") }},
		{"transaction hash", readerOverride{LedgerReader: authority, mutateReceipt: func(v *ledger.Receipt) { v.Hash[0]++ }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyAuthorityEvent(ctx, event.ID) }},
		{"block number", readerOverride{LedgerReader: authority, mutateReceipt: func(v *ledger.Receipt) { v.BlockNumber++ }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyAuthorityEvent(ctx, event.ID) }},
		{"block hash", readerOverride{LedgerReader: authority, mutateReceipt: func(v *ledger.Receipt) { v.BlockHash[0]++ }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyAuthorityEvent(ctx, event.ID) }},
		{"failed receipt", readerOverride{LedgerReader: authority, mutateReceipt: func(v *ledger.Receipt) { v.Status = ledger.ReceiptReverted }}, func(v *verify.Service) (verify.Result, error) { return v.VerifyAuthorityEvent(ctx, event.ID) }},
	}
	for _, test := range mismatchCases {
		t.Run(test.name, func(t *testing.T) {
			result, verifyErr := test.call(verify.New(store, test.reader))
			if verifyErr != nil || result.Status != verify.StatusFailed {
				t.Fatalf("result=%+v error=%v", result, verifyErr)
			}
		})
	}

	if _, err = verifier.VerifyAuthorityEvent(ctx, "MISSING"); !errors.Is(err, verify.ErrNotFound) {
		t.Fatalf("missing event %v", err)
	}
	if _, err = verifier.VerifyResponse(ctx, event.ID, "MISSING"); !errors.Is(err, verify.ErrNotFound) {
		t.Fatalf("missing response %v", err)
	}
	if _, err = verifier.VerifyAuthorityEvent(ctx, strings.Repeat("X", 33)); !errors.Is(err, verify.ErrInvalid) {
		t.Fatalf("invalid identifier %v", err)
	}

	handler := rc2http.Handler(workflowService, verifier)
	request := httptest.NewRequest(http.MethodGet, "/api/rc2/authority-events/EVENT-PHASE7/verification/bundle", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"VERIFIED"`) {
		t.Fatalf("HTTP bundle %d %s", response.Code, response.Body.String())
	}
	unavailableHandler := rc2http.Handler(workflowService, verify.New(store, unavailableReader{}))
	response = httptest.NewRecorder()
	unavailableHandler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable || !strings.Contains(response.Body.String(), `"status":"INDETERMINATE"`) {
		t.Fatalf("HTTP unavailable %d %s", response.Code, response.Body.String())
	}
	failedHandler := rc2http.Handler(workflowService, verify.New(store, readerOverride{LedgerReader: authority, mutateEvent: func(v *ledger.AuthorityAssertion) { v.EventCommitment[0]++ }}))
	failedRequest := httptest.NewRequest(http.MethodGet, "/api/rc2/authority-events/EVENT-PHASE7/verification", nil)
	response = httptest.NewRecorder()
	failedHandler.ServeHTTP(response, failedRequest)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"FAILED"`) {
		t.Fatalf("HTTP failed integrity %d %s", response.Code, response.Body.String())
	}

	after := snapshot(t, store)
	if before != after {
		t.Fatalf("verification mutated durable state: before=%+v after=%+v", before, after)
	}
}
