package ledger

import "testing"

func TestTypedIdentifierBoundary(t *testing.T) {
	id, err := NewID("HOSPITAL-A")
	if err != nil {
		t.Fatal(err)
	}
	if id.String() != "HOSPITAL-A" {
		t.Fatal(id.String())
	}
	for _, bad := range []string{"", "lowercase", "HAS SPACE", "ABCDEFGHIJKLMNOPQRSTUVWXYZ1234567"} {
		if _, err := NewID(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
}

func TestIdentityTypesAreDistinct(t *testing.T) {
	var _ ID
	var _ Address
	var _ Commitment
	var _ TransactionHash
}
