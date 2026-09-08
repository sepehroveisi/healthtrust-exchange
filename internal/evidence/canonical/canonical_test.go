package canonical

import (
	"bytes"
	"encoding/hex"
	"testing"
	"time"
)

func mustTime(value string) time.Time { return mustTimeT(nil, value) }
func mustTimeT(t *testing.T, value string) time.Time {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		if t != nil {
			t.Fatal(err)
		}
		panic(err)
	}
	return parsed
}

func TestPolicyGoldenVector(t *testing.T) {
	v := PolicyV1{"POLICY-HOSPITAL-A-EXCLUSION", "HOSPITAL-A", "V1", mustTime("2026-01-01T00:00:00Z"), nil, "Suspend scheduling while exclusion is active."}
	got, err := PolicyCommitment(v)
	if err != nil {
		t.Fatal(err)
	}
	const want = "2c8b64b3f633a38d1afddb020aff4add3b435c0974666ed8ff4821b868bb4d21"
	if hex.EncodeToString(got[:]) != want {
		t.Fatalf("golden mismatch: %x", got)
	}
}

func TestGoldenVectorsAllTypes(t *testing.T) {
	now := mustTime("2026-01-01T00:00:00.123456789Z")
	source := Hash([]byte("synthetic-source"))
	policy := Hash([]byte("policy"))
	decision := Hash([]byte("decision"))
	previous := "EVENT-0"
	cases := []struct {
		name, want string
		fn         func() (Commitment, error)
	}{
		{"authority-event", "", func() (Commitment, error) {
			return AuthorityEventCommitment(AuthorityEventV1{"EVENT-1", "SERIES-1", &previous, &previous, "HHS-OIG-DEMO", "PRV-7F31A", 1, 2, 1, now, source})
		}},
		{"decision", "", func() (Commitment, error) {
			return DecisionCommitment(DecisionV1{"DECISION-1", "EVENT-1", "HOSPITAL-A", policy, "HOLD", "Synthetic explanation", now, "REVIEWER-1"})
		}},
		{"action", "", func() (Commitment, error) {
			return ActionCommitment(ActionV1{"ACTION-1", "EVENT-1", "HOSPITAL-A", decision, "SUSPEND", "Synthetic action", nil, now, "OPERATOR-1"})
		}},
		{"response", "", func() (Commitment, error) {
			return ResponseCommitment(ResponseV1{"RESPONSE-1", "RESPONSE-1-V1", nil, "EVENT-1", "HOSPITAL-A", 1, now, nil, nil, nil})
		}},
	}
	// These constants are populated independently from the canonical byte format and freeze V1.
	wants := map[string]string{
		"authority-event": "97690a8630c4b9003e82b1d151ac856af6458e45dfbaf47751b0414b6b464d03",
		"decision":        "f8a7a8fca055edc4f623f5bcf73aca9fce57185f9de57c5879cbe6df58744bd0",
		"action":          "57f7f644d1b839596102d93e254f805cc8be79cefb8fe28d2ca6a9d66a6cd114",
		"response":        "9b1a9976e68d5e053f0d131fdc667b46143e7bc56cbc8db1ef5a8b304c83e88e",
	}
	for _, tc := range cases {
		got, err := tc.fn()
		if err != nil {
			t.Fatal(err)
		}
		tc.want = wants[tc.name]
		if hex.EncodeToString(got[:]) != tc.want {
			t.Errorf("%s: got %s want %s", tc.name, hex.EncodeToString(got[:]), tc.want)
		}
	}
}

func TestDeterminismMutationAndDomainSeparation(t *testing.T) {
	v := PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", mustTime("2026-01-01T00:00:00Z"), nil, "abc"}
	a, _ := PolicyCanonicalBytes(v)
	b, _ := PolicyCanonicalBytes(v)
	if !bytes.Equal(a, b) {
		t.Fatal("not deterministic")
	}
	v.PolicyText = "abd"
	c, _ := PolicyCanonicalBytes(v)
	if bytes.Equal(a, c) {
		t.Fatal("mutation did not change bytes")
	}
	if a[6] == 1 {
		t.Fatal("policy domain separator equals event")
	}
}

func TestUnicodeNFCAndNormalizationBoundary(t *testing.T) {
	decomposed := "Cafe\u0301"
	normalized, err := NormalizeText(decomposed)
	if err != nil {
		t.Fatal(err)
	}
	if normalized != "Café" {
		t.Fatalf("got %q", normalized)
	}
	v := PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", mustTime("2026-01-01T00:00:00Z"), nil, decomposed}
	if _, err := PolicyCanonicalBytes(v); err == nil {
		t.Fatal("non-NFC text accepted")
	}
	v.PolicyText = normalized
	if _, err := PolicyCanonicalBytes(v); err != nil {
		t.Fatal(err)
	}
}

func TestNullEmptyTimestampEnumsAndValidation(t *testing.T) {
	now := mustTime("2026-01-01T00:00:00Z")
	empty := ""
	a, _ := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", now, nil, ""})
	b, err := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", now, &now, ""})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("null and present timestamp equal")
	}
	r := ResponseV1{"RESPONSE-1", "VERSION-1", nil, "EVENT-1", "HOSPITAL-A", 1, now, nil, nil, nil}
	x, _ := ResponseCanonicalBytes(r)
	r.PreviousResponseVersionID = &empty
	if _, err := ResponseCanonicalBytes(r); err == nil {
		t.Fatal("present empty identifier accepted")
	}
	_ = x
	r.PreviousResponseVersionID = nil
	r.ResponseState = 99
	if _, err := ResponseCanonicalBytes(r); err == nil {
		t.Fatal("invalid enum accepted")
	}
	if _, err := ParseCommitment(make([]byte, 31)); err == nil {
		t.Fatal("wrong hash size accepted")
	}
	bad := PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", time.Time{}, nil, "x"}
	if _, err := PolicyCanonicalBytes(bad); err == nil {
		t.Fatal("zero timestamp accepted")
	}
}

func TestTimestampUTCEquivalenceNanosecondsAndLengthPrefixes(t *testing.T) {
	utc := mustTime("2026-01-01T00:00:00.000000001Z")
	offset := utc.In(time.FixedZone("offset", 3*3600))
	a, _ := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", utc, nil, "ab"})
	b, _ := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", offset, nil, "ab"})
	if !bytes.Equal(a, b) {
		t.Fatal("same instant encoded differently")
	}
	c, _ := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", utc.Add(time.Nanosecond), nil, "ab"})
	if bytes.Equal(a, c) {
		t.Fatal("nanosecond lost")
	}
	d, _ := PolicyCanonicalBytes(PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", utc, nil, "a"})
	if bytes.Equal(a, d) {
		t.Fatal("length prefix ambiguity")
	}
}
