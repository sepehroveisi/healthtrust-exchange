package authorityledger

import (
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
)

func reject(t *testing.T, s *Store, ctx context.Context, name, sql string, args ...any) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		if _, err := s.pool.Exec(ctx, sql, args...); err == nil {
			t.Fatal("database accepted invalid row")
		}
	})
}

const assertionInsert = `INSERT INTO authority_ledger.authority_assertions(id,series_id,previous_id,target_id,event_type,assertion_kind,effect,effective_unix_nano,source_document,commitment,canonical_version) VALUES($1,$2,$3,$4,'EXCLUSION',$5,'EXCLUSION_ACTIVE',1,'x',$6,1)`
const submissionInsert = `INSERT INTO authority_ledger.ledger_submissions(operation_id,operation_type,domain_version_id,contract_action,signer_context,intended_commitment,state,transaction_hash,block_number,block_hash) VALUES($1,'AUTHORITY_ASSERTION',$2,'recordAssertion','HHS-OIG-DEMO',$3,$4,$5,$6,$7)`

func TestCompleteConstraintMatrix(t *testing.T) {
	ctx, s := testStore(t)
	seed(t, ctx, s)
	if err := s.AppendAssertion(ctx, original()); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateSubject(ctx, ProfessionalSubject{"PRV-2", "Other", []byte(`{}`)}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateEventSeries(ctx, EventSeries{"SERIES-2", "PRV-2", "HHS-OIG-DEMO"}); err != nil {
		t.Fatal(err)
	}
	root2 := original()
	root2.ID = "EVENT-X"
	root2.SeriesID = "SERIES-2"
	if err := s.AppendAssertion(ctx, root2); err != nil {
		t.Fatal(err)
	}
	e1 := AssertionID("EVENT-1")
	later := original()
	later.ID = "EVENT-2"
	later.PreviousID = &e1
	later.TargetID = &e1
	later.Kind = "CORRECTION"
	if err := s.AppendAssertion(ctx, later); err != nil {
		t.Fatal(err)
	}
	h := bytes.Repeat([]byte{1}, 32)
	reject(t, s, ctx, "second authority root", assertionInsert, "ROOT-2", "SERIES-1", nil, nil, "ORIGINAL", h)
	reject(t, s, ctx, "second successor", assertionInsert, "EVENT-3", "SERIES-1", "EVENT-1", "EVENT-1", "CORRECTION", h)
	reject(t, s, ctx, "cross-series predecessor", assertionInsert, "BAD-PREV", "SERIES-1", "EVENT-X", "EVENT-1", "CORRECTION", h)
	reject(t, s, ctx, "cross-series target", assertionInsert, "BAD-TARGET", "SERIES-1", "EVENT-2", "EVENT-X", "CORRECTION", h)
	reject(t, s, ctx, "original predecessor", assertionInsert, "BAD-O-P", "SERIES-1", "EVENT-2", nil, "ORIGINAL", h)
	reject(t, s, ctx, "original target", assertionInsert, "BAD-O-T", "SERIES-1", nil, "EVENT-1", "ORIGINAL", h)
	reject(t, s, ctx, "non-original no predecessor", assertionInsert, "BAD-N-P", "SERIES-1", nil, "EVENT-1", "CORRECTION", h)
	reject(t, s, ctx, "non-original no target", assertionInsert, "BAD-N-T", "SERIES-1", "EVENT-2", nil, "CORRECTION", h)
	reject(t, s, ctx, "assertion commitment length", assertionInsert, "BAD-H", "SERIES-2", "EVENT-X", "EVENT-X", "CORRECTION", []byte{1})
	if err := s.CreateResponseStream(ctx, ResponseStream{"R1", "EVENT-1", "HOSPITAL-A"}); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateResponseStream(ctx, ResponseStream{"R2", "EVENT-1", "PAYER-B"}); err != nil {
		t.Fatal(err)
	}
	r1 := ResponseVersion{ID: "R1-V1", ResponseID: "R1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: 1, Commitment: hash("r1"), CanonicalVersion: 1}
	if err := s.AppendResponse(ctx, r1); err != nil {
		t.Fatal(err)
	}
	r1id := r1.ID
	r2 := r1
	r2.ID = "R1-V2"
	r2.PreviousID = &r1id
	r2.State = "UNDER_REVIEW"
	if err := s.AppendResponse(ctx, r2); err != nil {
		t.Fatal(err)
	}
	if err := s.AppendResponse(ctx, ResponseVersion{ID: "R2-V1", ResponseID: "R2", EventID: "EVENT-1", OrganizationID: "PAYER-B", State: "RECEIVED", ReceiptUnixNano: 1, Commitment: hash("r2"), CanonicalVersion: 1}); err != nil {
		t.Fatal(err)
	}
	reject(t, s, ctx, "second stream", `INSERT INTO authority_ledger.response_streams VALUES('R3','EVENT-1','HOSPITAL-A',now())`)
	const ri = `INSERT INTO authority_ledger.response_versions(version_id,response_id,event_id,organization_id,previous_version_id,state,receipt_unix_nano,commitment,canonical_version) VALUES($1,$2,$3,$4,$5,'RECEIVED',1,$6,1)`
	reject(t, s, ctx, "second response root", ri, "R1-ROOT2", "R1", "EVENT-1", "HOSPITAL-A", nil, h)
	reject(t, s, ctx, "second response successor", ri, "R1-V3", "R1", "EVENT-1", "HOSPITAL-A", "R1-V1", h)
	reject(t, s, ctx, "cross-stream predecessor", ri, "R2-BAD", "R2", "EVENT-1", "PAYER-B", "R1-V2", h)
	reject(t, s, ctx, "response commitment length", ri, "R2-V2", "R2", "EVENT-1", "PAYER-B", "R2-V1", []byte{1})
	reject(t, s, ctx, "organization signer", `INSERT INTO authority_ledger.organizations VALUES('BAD','HOSPITAL','bad',true,'x',now())`)
	reject(t, s, ctx, "authority signer", `INSERT INTO authority_ledger.authorities VALUES('BAD','EXCLUSION_AUTHORITY','bad',true,'x',now())`)
	reject(t, s, ctx, "empty operation", submissionInsert, "", "D0", h, "LOCAL_PENDING", nil, nil, nil)
	if _, err := s.pool.Exec(ctx, submissionInsert, "OP1", "D1", h, "LOCAL_PENDING", nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	reject(t, s, ctx, "duplicate operation", submissionInsert, "OP1", "D2", h, "LOCAL_PENDING", nil, nil, nil)
	reject(t, s, ctx, "duplicate logical", submissionInsert, "OP2", "D1", h, "LOCAL_PENDING", nil, nil, nil)
	reject(t, s, ctx, "submission commitment", submissionInsert, "OP3", "D3", []byte{1}, "LOCAL_PENDING", nil, nil, nil)
	cases := []struct {
		name, state string
		tx          []byte
		block       *int64
		bh          []byte
	}{{"pending tx", "LOCAL_PENDING", h, nil, nil}, {"pending block", "LOCAL_PENDING", nil, ptr64(1), nil}, {"pending hash", "LOCAL_PENDING", nil, nil, h}, {"submitted no tx", "SUBMITTED", nil, nil, nil}, {"submitted block", "SUBMITTED", h, ptr64(1), nil}, {"submitted hash", "SUBMITTED", h, nil, h}, {"confirmed no tx", "CONFIRMED", nil, ptr64(1), h}, {"confirmed no block", "CONFIRMED", h, nil, h}, {"confirmed no hash", "CONFIRMED", h, ptr64(1), nil}, {"failed block", "SUBMISSION_FAILED", nil, ptr64(1), nil}, {"failed hash", "SUBMISSION_FAILED", nil, nil, h}, {"negative block", "CONFIRMED", h, ptr64(-1), h}}
	for i, c := range cases {
		reject(t, s, ctx, c.name, submissionInsert, "OP-X-"+string(rune('A'+i)), "DX-"+string(rune('A'+i)), h, c.state, c.tx, c.block, c.bh)
	}
}
func ptr64(v int64) *int64 { return &v }

func TestFiveCanonicalRoundTrips(t *testing.T) {
	ctx, s := testStore(t)
	seed(t, ctx, s)
	now := time.Date(2026, 1, 1, 0, 0, 0, 123456789, time.UTC)
	source := canonical.Hash([]byte("synthetic-source"))
	event := canonical.AuthorityEventV1{EventID: "EVENT-1", EventSeriesID: "SERIES-1", AuthorityID: "HHS-OIG-DEMO", ProviderReference: "PRV-7F31A", EventType: 1, AssertionKind: 1, AuthorityEffect: 1, EffectiveTime: now, SourceDocumentHash: source}
	ec, _ := canonical.AuthorityEventCommitment(event)
	a := original()
	a.EffectiveUnixNano = now.UnixNano()
	copy(a.Commitment[:], ec[:])
	if err := s.AppendAssertion(ctx, a); err != nil {
		t.Fatal(err)
	}
	ga, _ := s.GetAssertion(ctx, a.ID)
	assertCanonical(t, ga.Commitment, ec, ga.EffectiveUnixNano, now.UnixNano(), ga.CanonicalVersion)
	policy := canonical.PolicyV1{PolicyID: "POLICY-HOSPITAL-A-EXCLUSION", OrganizationID: "HOSPITAL-A", PolicyVersion: "V1", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), PolicyText: "Suspend scheduling while exclusion is active."}
	pc, _ := canonical.PolicyCommitment(policy)
	p := Policy{policy.PolicyID, "POLICY-V1", "HOSPITAL-A", policy.EffectiveFrom.UnixNano(), nil, policy.PolicyText, Commitment(pc), 1}
	if err := s.CreatePolicy(ctx, p); err != nil {
		t.Fatal(err)
	}
	gp, _ := s.GetPolicy(ctx, p.VersionID)
	assertCanonical(t, gp.Commitment, pc, gp.EffectiveFromUnixNano, p.EffectiveFromUnixNano, gp.CanonicalVersion)
	dv := canonical.DecisionV1{DecisionID: "DECISION-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", PolicyVersionHash: pc, DecisionCode: "HOLD", DecisionExplanation: "Synthetic explanation", DecidedAt: now, ReviewerReference: "REVIEWER-1"}
	dc, _ := canonical.DecisionCommitment(dv)
	d := Decision{dv.DecisionID, "EVENT-1", "HOSPITAL-A", p.VersionID, dv.DecisionCode, dv.DecisionExplanation, dv.ReviewerReference, now.UnixNano(), Commitment(dc), 1}
	if err := s.CreateDecision(ctx, d); err != nil {
		t.Fatal(err)
	}
	gd, _ := s.GetDecision(ctx, d.ID)
	assertCanonical(t, gd.Commitment, dc, gd.DecidedUnixNano, now.UnixNano(), gd.CanonicalVersion)
	av := canonical.ActionV1{ActionID: "ACTION-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", DecisionHash: dc, ActionCode: "SUSPEND", ActionExplanation: "Synthetic action", CompletedAt: now, OperatorReference: "OPERATOR-1"}
	ac, _ := canonical.ActionCommitment(av)
	act := Action{av.ActionID, "EVENT-1", "HOSPITAL-A", d.ID, av.ActionCode, av.ActionExplanation, av.OperatorReference, now.UnixNano(), nil, Commitment(ac), 1}
	if err := s.CreateAction(ctx, act); err != nil {
		t.Fatal(err)
	}
	gact, _ := s.GetAction(ctx, act.ID)
	assertCanonical(t, gact.Commitment, ac, gact.CompletedUnixNano, now.UnixNano(), gact.CanonicalVersion)
	if err := s.CreateResponseStream(ctx, ResponseStream{"RESPONSE-1", "EVENT-1", "HOSPITAL-A"}); err != nil {
		t.Fatal(err)
	}
	rv := canonical.ResponseV1{ResponseID: "RESPONSE-1", ResponseVersionID: "RESPONSE-1-V1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", ResponseState: 1, ReceiptTimestamp: now}
	rc, _ := canonical.ResponseCommitment(rv)
	r := ResponseVersion{ID: "RESPONSE-1-V1", ResponseID: "RESPONSE-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: now.UnixNano(), Commitment: Commitment(rc), CanonicalVersion: 1}
	if err := s.AppendResponse(ctx, r); err != nil {
		t.Fatal(err)
	}
	gr, _ := s.GetResponse(ctx, r.ID)
	assertCanonical(t, gr.Commitment, rc, gr.ReceiptUnixNano, now.UnixNano(), gr.CanonicalVersion)
	if hex.EncodeToString(pc[:]) != "2c8b64b3f633a38d1afddb020aff4add3b435c0974666ed8ff4821b868bb4d21" {
		t.Fatal("Policy golden changed")
	}
}
func assertCanonical(t *testing.T, got Commitment, want canonical.Commitment, gotN, wantN int64, v int16) {
	t.Helper()
	if !bytes.Equal(got[:], want[:]) || gotN != wantN || v != 1 {
		t.Fatalf("canonical round trip mismatch %x %x %d %d v%d", got, want, gotN, wantN, v)
	}
}

func TestResponseCompatibilityAndIndependentOrganizations(t *testing.T) {
	ctx, s := testStore(t)
	seed(t, ctx, s)
	if err := s.AppendAssertion(ctx, original()); err != nil {
		t.Fatal(err)
	}
	for i, org := range []OrganizationID{"HOSPITAL-A", "PAYER-B", "STAFFING-C"} {
		suffix := string(rune('A' + i))
		p := Policy{"P" + suffix, "PV" + suffix, org, 1, nil, "distinct " + suffix, hash("p" + suffix), 1}
		if err := s.CreatePolicy(ctx, p); err != nil {
			t.Fatal(err)
		}
		d := Decision{"D" + suffix, "EVENT-1", org, p.VersionID, "CODE-" + suffix, "decision " + suffix, "REVIEWER-" + suffix, 2, hash("d" + suffix), 1}
		if err := s.CreateDecision(ctx, d); err != nil {
			t.Fatal(err)
		}
		a := Action{"A" + suffix, "EVENT-1", org, d.ID, "ACTION-" + suffix, "action " + suffix, "OPERATOR-" + suffix, 3, []byte("support-" + suffix), hash("a" + suffix), 1}
		if err := s.CreateAction(ctx, a); err != nil {
			t.Fatal(err)
		}
		rid := ResponseID("R" + suffix)
		if err := s.CreateResponseStream(ctx, ResponseStream{ID: rid, EventID: "EVENT-1", OrganizationID: org}); err != nil {
			t.Fatal(err)
		}
		pid, did, aid := p.VersionID, d.ID, a.ID
		if err := s.AppendResponse(ctx, ResponseVersion{ID: ResponseVersionID("RV" + suffix), ResponseID: rid, EventID: "EVENT-1", OrganizationID: org, State: "ACTION_COMPLETED", ReceiptUnixNano: 4, PolicyVersionID: &pid, DecisionID: &did, ActionID: &aid, Commitment: hash("r" + suffix), CanonicalVersion: 1}); err != nil {
			t.Fatal(err)
		}
	}
	wrong := "PVB"
	r := ResponseVersion{ID: "BAD-RH-V1", ResponseID: "RA", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: 1, PolicyVersionID: &wrong, Commitment: hash("r"), CanonicalVersion: 1}
	if err := s.AppendResponse(ctx, r); err == nil || !strings.Contains(err.Error(), "policy organization") {
		t.Fatalf("wrong policy accepted: %v", err)
	}
	dec := "DB"
	r.PolicyVersionID = nil
	r.DecisionID = &dec
	if err := s.AppendResponse(ctx, r); err == nil {
		t.Fatal("wrong decision accepted")
	}
	act := "AB"
	r.DecisionID = &dec
	r.ActionID = &act
	if err := s.AppendResponse(ctx, r); err == nil {
		t.Fatal("wrong action accepted")
	}
}

func TestPrivacyBoundary(t *testing.T) {
	for _, typ := range []reflect.Type{reflect.TypeOf(ledger.AuthorityAssertion{}), reflect.TypeOf(ledger.ResponseVersion{})} {
		for i := 0; i < typ.NumField(); i++ {
			name := strings.ToLower(typ.Field(i).Name)
			for _, bad := range []string{"display", "detail", "document", "policytext", "explanation", "reviewer", "operator", "supporting"} {
				if strings.Contains(name, bad) {
					t.Fatalf("off-chain field %s leaked into %s", name, typ.Name())
				}
			}
		}
	}
	for _, p := range []string{filepath.Join("..", "..", "..", "contracts", "src", "AuthorityEventRegistry.sol"), filepath.Join("..", "..", "..", "contracts", "src", "ResponseLedger.sol")} {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, bad := range []string{"displayName", "policyText", "decisionExplanation", "reviewerReference", "operatorReference", "supportingEvidence"} {
			if bytes.Contains(b, []byte(bad)) {
				t.Fatalf("%s leaked into %s", bad, p)
			}
		}
	}
}
