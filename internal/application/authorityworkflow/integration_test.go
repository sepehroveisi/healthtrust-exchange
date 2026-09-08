//go:build integration

package authorityworkflow_test

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
	app "github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger/besu"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/transport/rc2http"
	"github.com/jackc/pgx/v5"
)

const (
	authorityKey = "00000000000000000000000000000000000000000000000000000000000a11ce"
	hospitalKey  = "0000000000000000000000000000000000000000000000000000000000000b0b"
	payerKey     = "000000000000000000000000000000000000000000000000000000000000ca11"
	staffingKey  = "0000000000000000000000000000000000000000000000000000000000c0ffee"
)

var contracts = besu.ContractAddresses{OrganizationRegistry: common.HexToAddress("0x1148e16d870cc191b97774d1b55f65d42ecf6cd7"), AuthorityRegistry: common.HexToAddress("0xf5495ad34a7f25b26bb8b8ef512c44c7cb5cc005"), AuthorityEventRegistry: common.HexToAddress("0x3372e3af8edce113af8926a924f823edf2a301c2"), ResponseLedger: common.HexToAddress("0xaebe3a5fc1b9049970d0a53967d07c98211bcde7")}

func newLedger(t *testing.T, rpc, key string) *besu.Adapter {
	t.Helper()
	v, e := besu.New(context.Background(), besu.Config{RPCURL: rpc, ExpectedChainID: big.NewInt(202603), Contracts: contracts, PrivateKeyHex: key, ReceiptTimeout: 15 * time.Second, PollInterval: 100 * time.Millisecond})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(v.Close)
	return v
}

type delayedSubmitter struct {
	delegate  *submission.Service
	operation authorityledger.OperationID
	failed    bool
}

func (d *delayedSubmitter) Submit(ctx context.Context, id authorityledger.OperationID) error {
	if id == d.operation && !d.failed {
		d.failed = true
		return ledger.ErrReceiptTimeout
	}
	return d.delegate.Submit(ctx, id)
}

func TestPhase6ApplicationWorkflowPostgresBesuHTTP(t *testing.T) {
	url := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	ctx := context.Background()
	store, e := authorityledger.Open(ctx, url)
	if e != nil {
		t.Fatal(e)
	}
	m := authorityledger.NewMigrator(store.Pool(), filepath.Join("..", "..", "..", "migrations", "authority_ledger"))
	_ = m.DownAll(ctx)
	if e = m.Up(ctx); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = m.DownAll(ctx); store.Close() })
	var address authorityledger.Address
	for _, org := range []authorityledger.Organization{{ID: "HOSPITAL-A", Type: "HOSPITAL", DisplayName: "Hospital A", Active: true, Signer: address}, {ID: "PAYER-B", Type: "PAYER", DisplayName: "Payer B", Active: true, Signer: address}, {ID: "STAFFING-AGENCY-C", Type: "STAFFING_AGENCY", DisplayName: "Staffing Agency C", Active: true, Signer: address}} {
		if e = store.CreateOrganization(ctx, org); e != nil {
			t.Fatal(e)
		}
	}
	if e = store.CreateAuthority(ctx, authorityledger.Authority{ID: "HHS-OIG-DEMO", Type: "EXCLUSION_AUTHORITY", DisplayName: "Synthetic authority", Active: true, Signer: address}); e != nil {
		t.Fatal(e)
	}
	if e = store.CreateSubject(ctx, authorityledger.ProfessionalSubject{ID: "PRV-7F31A", DisplayName: "Synthetic professional", Details: []byte(`{"synthetic":true}`)}); e != nil {
		t.Fatal(e)
	}
	authority := newLedger(t, "http://127.0.0.1:8545", authorityKey)
	hospital := newLedger(t, "http://127.0.0.1:8545", hospitalKey)
	payer := newLedger(t, "http://127.0.0.1:8545", payerKey)
	staffing := newLedger(t, "http://127.0.0.1:8545", staffingKey)
	ledgers := map[string]ledger.ReconciliationInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing}
	normal := submission.New(store, map[string]ledger.SubmissionInterface{"HHS-OIG-DEMO": authority, "HOSPITAL-A": hospital, "PAYER-B": payer, "STAFFING-AGENCY-C": staffing})
	reconciler := submission.NewReconciler(store, ledgers)
	delayed := &delayedSubmitter{delegate: normal, operation: "OP-P6-STAFF-V1"}
	service := app.New(store, delayed, reconciler)
	eventTime := time.Date(2026, 5, 1, 0, 0, 0, 123, time.UTC)
	event, err := service.RegisterAuthorityEvent(ctx, app.AuthorityEventInput{EventID: "EVENT-PHASE6", SeriesID: "SERIES-PHASE6", AuthorityID: "HHS-OIG-DEMO", SubjectID: "PRV-7F31A", OperationID: "OP-P6-EVENT", EffectiveTime: eventTime, SourceDocumentHash: canonical.Hash([]byte("private source")), SourceDocument: []byte("private source")})
	if err != nil || event.Ledger.State != "CONFIRMED" {
		t.Fatalf("event %+v %v", event, err)
	}
	same, err := service.RegisterAuthorityEvent(ctx, app.AuthorityEventInput{EventID: "EVENT-PHASE6", SeriesID: "SERIES-PHASE6", AuthorityID: "HHS-OIG-DEMO", SubjectID: "PRV-7F31A", OperationID: "DIFFERENT-OP", EffectiveTime: eventTime, SourceDocumentHash: canonical.Hash([]byte("private source")), SourceDocument: []byte("private source")})
	if err != nil || same.ID != event.ID {
		t.Fatalf("idempotent event %+v %v", same, err)
	}
	_, err = service.RegisterAuthorityEvent(ctx, app.AuthorityEventInput{EventID: "EVENT-PHASE6", SeriesID: "SERIES-PHASE6", AuthorityID: "HHS-OIG-DEMO", SubjectID: "PRV-7F31A", OperationID: "CONFLICT", EffectiveTime: eventTime, SourceDocumentHash: canonical.Hash([]byte("different"))})
	if !errors.Is(err, app.ErrConflict) {
		t.Fatalf("event conflict %v", err)
	}
	badStream := authorityledger.ResponseStream{ID: "P6-ROLLBACK", EventID: authorityledger.AssertionID(event.ID), OrganizationID: "HOSPITAL-A"}
	badVersion := authorityledger.ResponseVersion{ID: "P6-ROLLBACK-V1", ResponseID: badStream.ID, EventID: badStream.EventID, OrganizationID: badStream.OrganizationID, State: "RECEIVED", ReceiptUnixNano: eventTime.UnixNano(), CanonicalVersion: 1}
	badPending := authorityledger.Submission{OperationID: "OP-P6-ROLLBACK", State: "INVALID"}
	if err = store.CreateResponseStartWithPending(ctx, badStream, badVersion, badPending); err == nil {
		t.Fatal("expected response-start persistence failure")
	}
	if _, err = store.GetResponseStreamForEvent(ctx, badStream.EventID, badStream.OrganizationID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed response start leaked stream: %v", err)
	}
	if _, err = store.GetResponse(ctx, badVersion.ID); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("failed response start leaked version: %v", err)
	}
	type flow struct {
		org, short, response string
		receipt              time.Time
	}
	flows := []flow{{"HOSPITAL-A", "HOSP", "P6-HOSP", eventTime.Add(time.Hour)}, {"PAYER-B", "PAYER", "P6-PAYER", eventTime.Add(2 * time.Hour)}, {"STAFFING-AGENCY-C", "STAFF", "P6-STAFF", eventTime.Add(3 * time.Hour)}}
	if _, err = service.StartOrganizationResponse(ctx, app.StartResponseInput{ActorOrganizationID: "MISSING-ORG", OrganizationID: "MISSING-ORG", EventID: event.ID, ResponseID: "MISSING-RESPONSE", VersionID: "MISSING-V1", OperationID: "MISSING-OP", ReceiptTime: eventTime}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing organization %v", err)
	}
	if _, err = service.RegisterAuthorityEvent(ctx, app.AuthorityEventInput{EventID: strings.Repeat("X", 33), SeriesID: "SERIES-LONG", AuthorityID: "HHS-OIG-DEMO", SubjectID: "PRV-7F31A", OperationID: "OP-LONG", EffectiveTime: eventTime}); !errors.Is(err, app.ErrInvalid) {
		t.Fatalf("unsupported identifier length %v", err)
	}
	for _, f := range flows {
		start, submitErr := service.StartOrganizationResponse(ctx, app.StartResponseInput{ActorOrganizationID: f.org, OrganizationID: f.org, EventID: event.ID, ResponseID: f.response, VersionID: f.response + "-V1", OperationID: "OP-P6-" + f.short + "-V1", ReceiptTime: f.receipt})
		if f.org == "STAFFING-AGENCY-C" {
			if !errors.Is(submitErr, ledger.ErrReceiptTimeout) || start.Ledger.State != "LOCAL_PENDING" {
				t.Fatalf("durable delayed start %+v %v", start, submitErr)
			}
			if out, reconcileErr := service.ReconcileOperation(ctx, "OP-P6-STAFF-V1"); reconcileErr != nil || out != submission.OutcomeSubmitted {
				t.Fatalf("reconcile %s %v", out, reconcileErr)
			}
		} else if submitErr != nil || start.Ledger.State != "CONFIRMED" {
			t.Fatalf("start %s %+v %v", f.org, start, submitErr)
		}
		again, againErr := service.StartOrganizationResponse(ctx, app.StartResponseInput{ActorOrganizationID: f.org, OrganizationID: f.org, EventID: event.ID, ResponseID: f.response, VersionID: f.response + "-V1", OperationID: "IGNORED", ReceiptTime: f.receipt})
		if againErr != nil || again.ResponseID != f.response {
			t.Fatalf("duplicate start %s %v", f.org, againErr)
		}
		if _, conflictErr := service.StartOrganizationResponse(ctx, app.StartResponseInput{ActorOrganizationID: f.org, OrganizationID: f.org, EventID: event.ID, ResponseID: f.response, VersionID: f.response + "-CONFLICT", OperationID: "CONFLICT", ReceiptTime: f.receipt}); !errors.Is(conflictErr, app.ErrConflict) {
			t.Fatalf("conflicting duplicate start %s %v", f.org, conflictErr)
		}
		if _, transitionErr := service.AdvanceOrganizationResponse(ctx, app.AdvanceResponseInput{ActorOrganizationID: f.org, OrganizationID: f.org, EventID: event.ID, VersionID: f.response + "-SKIP", OperationID: "OP-P6-" + f.short + "-SKIP", TargetState: "DECIDED"}); !errors.Is(transitionErr, app.ErrConflict) {
			t.Fatalf("state skip %s %v", f.org, transitionErr)
		}
		states := []string{"UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"}
		for index, state := range states {
			n := index + 2
			input := app.AdvanceResponseInput{ActorOrganizationID: f.org, OrganizationID: f.org, EventID: event.ID, VersionID: f.response + "-V" + string(rune('0'+n)), OperationID: "OP-P6-" + f.short + "-V" + string(rune('0'+n)), TargetState: state, OccurredAt: f.receipt.Add(time.Duration(n) * time.Hour), PolicyID: "POL-" + f.short, PolicyVersionID: "POL-" + f.short + "-V1", PolicyText: "private " + f.short + " policy", DecisionID: "DEC-" + f.short, DecisionCode: "HOLD-" + f.short, DecisionExplanation: "private " + f.short + " decision", ReviewerReference: "REVIEW-" + f.short, ActionID: "ACT-" + f.short, ActionCode: "COMPLETE-" + f.short, ActionExplanation: "private " + f.short + " action", OperatorReference: "OPERATOR-" + f.short, SupportingEvidence: []byte("private support")}
			view, advanceErr := service.AdvanceOrganizationResponse(ctx, input)
			if advanceErr != nil || view.Ledger.State != "CONFIRMED" {
				t.Fatalf("advance %s %s %+v %v", f.org, state, view, advanceErr)
			}
		}
		history, historyErr := service.GetResponseHistory(ctx, event.ID, f.org)
		if historyErr != nil || len(history) != 4 || history[3].State != "ACTION_COMPLETED" {
			t.Fatalf("history %s %+v %v", f.org, history, historyErr)
		}
		policy, policyErr := store.GetPolicy(ctx, *history[3].PolicyVersionID)
		decision, decisionErr := store.GetDecision(ctx, *history[3].DecisionID)
		action, actionErr := store.GetAction(ctx, *history[3].ActionID)
		if policyErr != nil || decisionErr != nil || actionErr != nil || policy.Text != "private "+f.short+" policy" || decision.Explanation != "private "+f.short+" decision" || action.Explanation != "private "+f.short+" action" {
			t.Fatalf("organization-local details %s: policy=%q decision=%q action=%q errors=%v/%v/%v", f.org, policy.Text, decision.Explanation, action.Explanation, policyErr, decisionErr, actionErr)
		}
	}
	if _, err = service.AdvanceOrganizationResponse(ctx, app.AdvanceResponseInput{ActorOrganizationID: "HOSPITAL-A", OrganizationID: "PAYER-B", EventID: event.ID, TargetState: "ACTION_COMPLETED", VersionID: "BAD", OperationID: "BAD"}); !errors.Is(err, app.ErrForbidden) {
		t.Fatalf("cross org %v", err)
	}
	if _, err = service.StartOrganizationResponse(ctx, app.StartResponseInput{ActorOrganizationID: "HOSPITAL-A", OrganizationID: "HOSPITAL-A", EventID: "MISSING", ResponseID: "X", VersionID: "X-V1", OperationID: "X", ReceiptTime: eventTime}); !errors.Is(err, app.ErrNotFound) {
		t.Fatalf("missing event %v", err)
	}
	responses, err := service.ListOrganizationResponsesForEvent(ctx, event.ID)
	if err != nil || len(responses) != 3 {
		t.Fatalf("responses %+v %v", responses, err)
	}
	peer := newLedger(t, "http://127.0.0.1:8546", authorityKey)
	chainEvent, err := peer.GetAuthorityAssertion(ctx, ledger.MustID(event.ID))
	if err != nil || chainEvent.EventCommitment != ledger.Commitment(event.Commitment) {
		t.Fatalf("peer event %+v %v", chainEvent, err)
	}
	handler := rc2http.Handler(service)
	request := httptest.NewRequest(http.MethodGet, "/api/rc2/authority-events/EVENT-PHASE6", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("HTTP read %d %s", response.Code, response.Body.String())
	}
	bad := httptest.NewRequest(http.MethodPost, "/api/rc2/authority-events/EVENT-PHASE6/responses/PAYER-B/advance", strings.NewReader(`{}`))
	bad.Header.Set("X-Organization-ID", "HOSPITAL-A")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, bad)
	if response.Code != http.StatusConflict {
		t.Fatalf("HTTP boundary %d", response.Code)
	}
}
