package authorityledger

import (
	"bytes"
	"context"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testStore(t *testing.T) (context.Context, *Store) {
	t.Helper()
	u := os.Getenv("AUTHORITY_LEDGER_TEST_DATABASE_URL")
	if u == "" {
		t.Skip("set AUTHORITY_LEDGER_TEST_DATABASE_URL")
	}
	c := context.Background()
	p, e := pgxpool.New(c, u)
	if e != nil {
		t.Fatal(e)
	}
	m := NewMigrator(p, filepath.Join("..", "..", "..", "migrations", "authority_ledger"))
	_ = m.DownAll(c)
	if e = m.Up(c); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = m.DownAll(c); p.Close() })
	return c, &Store{p}
}
func hash(s string) (c Commitment) { x := canonical.Hash([]byte(s)); copy(c[:], x[:]); return }
func addr(b byte) (a Address) {
	for i := range a {
		a[i] = b
	}
	return
}
func seed(t *testing.T, c context.Context, s *Store) {
	t.Helper()
	for _, o := range []Organization{{"HOSPITAL-A", "HOSPITAL", "Hospital A", true, addr(1)}, {"PAYER-B", "PAYER", "Payer B", true, addr(2)}, {"STAFFING-C", "STAFFING_AGENCY", "Staffing C", true, addr(3)}} {
		if e := s.CreateOrganization(c, o); e != nil {
			t.Fatal(e)
		}
	}
	if e := s.CreateAuthority(c, Authority{"HHS-OIG-DEMO", "EXCLUSION_AUTHORITY", "Synthetic authority", true, addr(4)}); e != nil {
		t.Fatal(e)
	}
	if e := s.CreateSubject(c, ProfessionalSubject{"PRV-7F31A", "Synthetic Professional", []byte(`{"specialty":"synthetic"}`)}); e != nil {
		t.Fatal(e)
	}
	if e := s.CreateEventSeries(c, EventSeries{"SERIES-1", "PRV-7F31A", "HHS-OIG-DEMO"}); e != nil {
		t.Fatal(e)
	}
}
func original() AuthorityAssertion {
	return AuthorityAssertion{ID: "EVENT-1", SeriesID: "SERIES-1", EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", EffectiveUnixNano: 1767225600000000001, SourceDocument: []byte("private synthetic source"), Commitment: hash("event"), CanonicalVersion: 1}
}

func TestMigrationsRealPostgres(t *testing.T) {
	c, s := testStore(t)
	if _, e := s.pool.Exec(c, `CREATE TABLE IF NOT EXISTS public.rc1_phase5a_sentinel(id text PRIMARY KEY); INSERT INTO public.rc1_phase5a_sentinel(id) VALUES('keep') ON CONFLICT DO NOTHING`); e != nil {
		t.Fatal(e)
	}
	m := NewMigrator(s.pool, filepath.Join("..", "..", "..", "migrations", "authority_ledger"))
	if e := m.Up(c); e != nil {
		t.Fatal(e)
	}
	var n int
	if e := s.pool.QueryRow(c, `SELECT count(*) FROM information_schema.tables WHERE table_schema='authority_ledger'`).Scan(&n); e != nil || n != 11 {
		t.Fatalf("tables=%d %v", n, e)
	}
	if e := s.pool.QueryRow(c, `SELECT count(*) FROM pg_indexes WHERE schemaname='authority_ledger' AND indexname IN ('authority_assertions_one_root','authority_assertions_one_successor','response_versions_one_root','response_versions_one_successor','ledger_submissions_unresolved')`).Scan(&n); e != nil || n != 5 {
		t.Fatalf("indexes=%d %v", n, e)
	}
	if e := s.pool.QueryRow(c, `SELECT count(*) FROM public.authority_ledger_migration_versions WHERE version=1`).Scan(&n); e != nil || n != 1 {
		t.Fatal(n, e)
	}
	if e := m.DownAll(c); e != nil {
		t.Fatal(e)
	}
	var exists bool
	s.pool.QueryRow(c, `SELECT EXISTS(SELECT 1 FROM pg_namespace WHERE nspname='authority_ledger')`).Scan(&exists)
	if exists {
		t.Fatal("schema survived down")
	}
	var sentinel string
	if e := s.pool.QueryRow(c, `SELECT id FROM public.rc1_phase5a_sentinel WHERE id='keep'`).Scan(&sentinel); e != nil || sentinel != "keep" {
		t.Fatalf("RC1 sentinel changed: %q %v", sentinel, e)
	}
	if e := m.Up(c); e != nil {
		t.Fatal(e)
	}
}

func TestRepositoryCanonicalPrivacyAndHistory(t *testing.T) {
	c, s := testStore(t)
	seed(t, c, s)
	o, e := s.GetOrganization(c, "HOSPITAL-A")
	if e != nil || o.Signer != addr(1) {
		t.Fatal(o, e)
	}
	a, e := s.GetAuthority(c, "HHS-OIG-DEMO")
	if e != nil || a.Signer != addr(4) {
		t.Fatal(a, e)
	}
	p, e := s.GetSubject(c, "PRV-7F31A")
	if e != nil || !bytes.Contains(p.Details, []byte("specialty")) {
		t.Fatal(p, e)
	}
	ev := canonical.AuthorityEventV1{EventID: "EVENT-1", EventSeriesID: "SERIES-1", AuthorityID: "HHS-OIG-DEMO", ProviderReference: "PRV-7F31A", EventType: 1, AssertionKind: 1, AuthorityEffect: 1, EffectiveTime: time.Unix(0, 1767225600000000001), SourceDocumentHash: canonical.Hash([]byte("source"))}
	ch, e := canonical.AuthorityEventCommitment(ev)
	if e != nil {
		t.Fatal(e)
	}
	x := original()
	copy(x.Commitment[:], ch[:])
	if e = s.AppendAssertion(c, x); e != nil {
		t.Fatal(e)
	}
	got, e := s.GetAssertion(c, x.ID)
	if e != nil || got.Commitment != x.Commitment || got.EffectiveUnixNano != x.EffectiveUnixNano {
		t.Fatal(got, e)
	}
	target := x.ID
	later := x
	later.ID = "EVENT-2"
	later.PreviousID = &target
	later.TargetID = &target
	later.Kind = "CORRECTION"
	later.Commitment = hash("later")
	if e = s.AppendAssertion(c, later); e != nil {
		t.Fatal(e)
	}
	h, _ := s.AssertionHistory(c, "SERIES-1")
	if len(h) != 2 || h[0].ID != "EVENT-1" {
		t.Fatal(h)
	}
	pol := Policy{"POLICY-1", "POLICY-1-V1", "HOSPITAL-A", 1767225600000000002, nil, "Private policy text", hash("policy"), 1}
	if e = s.CreatePolicy(c, pol); e != nil {
		t.Fatal(e)
	}
	pg, _ := s.GetPolicy(c, pol.VersionID)
	if pg.Commitment != pol.Commitment || pg.EffectiveFromUnixNano != pol.EffectiveFromUnixNano || pg.Text != pol.Text {
		t.Fatal(pg)
	}
	dec := Decision{"DECISION-1", "EVENT-1", "HOSPITAL-A", pol.VersionID, "HOLD", "Private explanation", "REVIEWER-1", 1767225600000000003, hash("decision"), 1}
	if e = s.CreateDecision(c, dec); e != nil {
		t.Fatal(e)
	}
	dg, _ := s.GetDecision(c, dec.ID)
	if dg.Commitment != dec.Commitment || dg.Explanation != dec.Explanation {
		t.Fatal(dg)
	}
	act := Action{"ACTION-1", "EVENT-1", "HOSPITAL-A", dec.ID, "SUSPEND", "Private action", "OPERATOR-1", 1767225600000000004, []byte("support"), hash("action"), 1}
	if e = s.CreateAction(c, act); e != nil {
		t.Fatal(e)
	}
	ag, _ := s.GetAction(c, act.ID)
	if ag.Commitment != act.Commitment || !bytes.Equal(ag.SupportingEvidence, act.SupportingEvidence) {
		t.Fatal(ag)
	}
	if e = s.CreateResponseStream(c, ResponseStream{"RESPONSE-1", "EVENT-1", "HOSPITAL-A"}); e != nil {
		t.Fatal(e)
	}
	rv := ResponseVersion{ID: "RESPONSE-1-V1", ResponseID: "RESPONSE-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: 1767225600000000005, Commitment: hash("response1"), CanonicalVersion: 1}
	if e = s.AppendResponse(c, rv); e != nil {
		t.Fatal(e)
	}
	pid := rv.ID
	rv2 := rv
	rv2.ID = "RESPONSE-1-V2"
	rv2.PreviousID = &pid
	rv2.State = "UNDER_REVIEW"
	rv2.PolicyVersionID = &pol.VersionID
	rv2.Commitment = hash("response2")
	if e = s.AppendResponse(c, rv2); e != nil {
		t.Fatal(e)
	}
	rh, _ := s.ResponseHistory(c, "RESPONSE-1")
	latest, _ := s.LatestResponse(c, "RESPONSE-1")
	if len(rh) != 2 || latest.ID != rv2.ID || rh[0].Commitment != rv.Commitment {
		t.Fatal(rh, latest)
	}
}

func TestAtomicPairs(t *testing.T) {
	c, s := testStore(t)
	seed(t, c, s)
	bad := Submission{"", "AUTHORITY_ASSERTION", "EVENT-1", "recordAssertion", "HHS-OIG-DEMO", hash("event"), "LOCAL_PENDING", nil, nil, nil, 0, nil, nil, time.Time{}, time.Time{}}
	if e := s.CreateAssertionWithPending(c, original(), bad); e == nil {
		t.Fatal("invalid submission accepted")
	}
	n, _ := s.Count(c, "authority_assertions")
	if n != 0 {
		t.Fatal("assertion survived rollback")
	}
	good := bad
	good.OperationID = "OP-EVENT-1"
	if e := s.CreateAssertionWithPending(c, original(), good); e != nil {
		t.Fatal(e)
	}
	n, _ = s.Count(c, "ledger_submissions")
	if n != 1 {
		t.Fatal(n)
	}
	if e := s.CreateResponseStream(c, ResponseStream{"RESPONSE-1", "EVENT-1", "HOSPITAL-A"}); e != nil {
		t.Fatal(e)
	}
	rv := ResponseVersion{ID: "RESPONSE-V1", ResponseID: "RESPONSE-1", EventID: "EVENT-1", OrganizationID: "HOSPITAL-A", State: "RECEIVED", ReceiptUnixNano: 1, Commitment: hash("r"), CanonicalVersion: 1}
	bad.DomainVersionID = string(rv.ID)
	if e := s.CreateResponseWithPending(c, rv, bad); e == nil {
		t.Fatal("invalid accepted")
	}
	n, _ = s.Count(c, "response_versions")
	if n != 0 {
		t.Fatal("response survived rollback")
	}
	good = bad
	good.OperationID = "OP-R1"
	good.OperationType = "RESPONSE_VERSION"
	if e := s.CreateResponseWithPending(c, rv, good); e != nil {
		t.Fatal(e)
	}
}

func TestDatabaseConstraints(t *testing.T) {
	c, s := testStore(t)
	seed(t, c, s)
	if e := s.AppendAssertion(c, original()); e != nil {
		t.Fatal(e)
	}
	tests := []string{
		`INSERT INTO authority_ledger.authority_assertions VALUES('ROOT2','SERIES-1',NULL,NULL,'EXCLUSION','ORIGINAL','EXCLUSION_ACTIVE',1,'x',repeat('a',32)::bytea,1,now())`,
		`INSERT INTO authority_ledger.organizations(id,type,display_name,active,signer_address) VALUES('BAD','HOSPITAL','bad',true,'x')`,
		`INSERT INTO authority_ledger.ledger_submissions(operation_id,operation_type,domain_version_id,contract_action,signer_context,intended_commitment,state) VALUES('X','AUTHORITY_ASSERTION','X','x','x',repeat('a',32)::bytea,'SUBMITTED')`,
		`INSERT INTO authority_ledger.ledger_submissions(operation_id,operation_type,domain_version_id,contract_action,signer_context,intended_commitment,state,transaction_hash,block_number,block_hash) VALUES('Y','AUTHORITY_ASSERTION','Y','x','x',repeat('a',32)::bytea,'CONFIRMED',repeat('b',32)::bytea,-1,repeat('c',32)::bytea)`,
	}
	for _, q := range tests {
		if _, e := s.pool.Exec(c, q); e == nil {
			t.Errorf("constraint accepted: %s", q)
		}
	}
}

func TestNoEmbeddedRC2Schema(t *testing.T) {
	matches, e := filepath.Glob("*.go")
	if e != nil {
		t.Fatal(e)
	}
	for _, p := range matches {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		b, _ := os.ReadFile(p)
		if bytes.Contains(b, []byte("CREATE TABLE authority_ledger")) {
			t.Fatalf("embedded schema in %s", p)
		}
	}
}

var _ = errors.Is
var _ = pgx.ErrNoRows
