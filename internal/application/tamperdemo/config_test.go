package tamperdemo

import "testing"

func TestNewUsesEnvironmentGateAndDefaultsDisabled(t *testing.T) {
	for _, test := range []struct {
		value string
		want  bool
	}{{"", false}, {"false", false}, {"TRUE", false}, {"true", true}} {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(EnvironmentVariable, test.value)
			if got := New(nil, nil, nil).enabled; got != test.want {
				t.Fatalf("service enabled=%v want=%v", got, test.want)
			}
		})
	}
}
