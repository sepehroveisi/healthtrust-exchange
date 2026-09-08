package canonical

import (
	"bytes"
	"testing"
	"time"
)

func TestEnumCodesRemainFrozen(t *testing.T) {
	if EventTypeExclusion != 1 || AssertionKindOriginal != 1 || AssertionKindCorrection != 2 ||
		AssertionKindSupersession != 3 || AssertionKindReinstatement != 4 ||
		AuthorityEffectExclusionActive != 1 || AuthorityEffectExclusionLifted != 2 ||
		ResponseStateReceived != 1 || ResponseStateUnderReview != 2 ||
		ResponseStateDecided != 3 || ResponseStateActionCompleted != 4 {
		t.Fatal("V1 enum codes changed")
	}
}

func TestInvalidUTF8AndOutOfRangeTimestampAreRejected(t *testing.T) {
	v := PolicyV1{"POLICY-1", "HOSPITAL-A", "V1", mustTime("2026-01-01T00:00:00Z"), nil, string([]byte{0xff})}
	if _, err := PolicyCanonicalBytes(v); err == nil {
		t.Fatal("invalid UTF-8 accepted")
	}
	v.PolicyText = "valid"
	v.EffectiveFrom = time.Date(2500, 1, 1, 0, 0, 0, 0, time.UTC)
	if _, err := PolicyCanonicalBytes(v); err == nil {
		t.Fatal("timestamp outside signed Unix-nanosecond range accepted")
	}
}

func TestNullableHashDiffersFromPresentZeroHash(t *testing.T) {
	now := mustTime("2026-01-01T00:00:00Z")
	v := ResponseV1{"RESPONSE-1", "VERSION-1", nil, "EVENT-1", "HOSPITAL-A", ResponseStateReceived, now, nil, nil, nil}
	nullBytes, err := ResponseCanonicalBytes(v)
	if err != nil {
		t.Fatal(err)
	}
	zero := Commitment{}
	v.PolicyVersionHash = &zero
	presentBytes, err := ResponseCanonicalBytes(v)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(nullBytes, presentBytes) {
		t.Fatal("null and present zero hash encoded identically")
	}
}
