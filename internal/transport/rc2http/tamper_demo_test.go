package rc2http

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/tamperdemo"
)

type fakeTamperDemo struct {
	result tamperdemo.Result
	err    error
	calls  int
}

func (f *fakeTamperDemo) Demonstrate(context.Context, tamperdemo.Target) (tamperdemo.Result, error) {
	f.calls++
	return f.result, f.err
}

func requestDemo(t *testing.T, demo TamperDemoService, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/rc2/demo/tamper", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	HandlerWithDemo(nil, nil, demo).ServeHTTP(recorder, request)
	return recorder
}

func TestTamperDemoHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, http.StatusOK},
		{"disabled", tamperdemo.ErrDemoTamperDisabled, http.StatusForbidden},
		{"unsupported", tamperdemo.ErrUnsupportedTarget, http.StatusBadRequest},
		{"missing", tamperdemo.ErrSyntheticFixtureMissing, http.StatusNotFound},
		{"precondition", tamperdemo.ErrPrecondition, http.StatusConflict},
		{"fixture mismatch", tamperdemo.ErrSyntheticFixtureMismatch, http.StatusConflict},
		{"unavailable", tamperdemo.ErrVerificationUnavailable, http.StatusServiceUnavailable},
		{"mutation failed", tamperdemo.ErrMutationFailed, http.StatusInternalServerError},
		{"postcondition failed", tamperdemo.ErrPostcondition, http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			demo := &fakeTamperDemo{result: tamperdemo.Result{DemoOnly: true, Warning: tamperdemo.DemoWarning}, err: test.err}
			response := requestDemo(t, demo, `{"target":"AUTHORITY_EVENT"}`)
			if response.Code != test.want || demo.calls != 1 {
				t.Fatalf("status=%d want=%d calls=%d body=%s", response.Code, test.want, demo.calls, response.Body.String())
			}
			if test.err == nil && (!strings.Contains(response.Body.String(), `"demoOnly":true`) || !strings.Contains(response.Body.String(), tamperdemo.DemoWarning)) {
				t.Fatalf("success response lacks demo markers: %s", response.Body.String())
			}
		})
	}
}

func TestTamperDemoHTTPRejectsArbitraryInput(t *testing.T) {
	for _, body := range []string{`{"target":"AUTHORITY_EVENT","sql":"UPDATE policies"}`, `{"target":{"table":"policies"}}`, `not-json`} {
		demo := &fakeTamperDemo{err: errors.New("must not be called")}
		response := requestDemo(t, demo, body)
		if response.Code != http.StatusBadRequest || demo.calls != 0 {
			t.Fatalf("body=%s status=%d calls=%d response=%s", body, response.Code, demo.calls, response.Body.String())
		}
	}
	demo := &fakeTamperDemo{err: tamperdemo.ErrUnsupportedTarget}
	response := requestDemo(t, demo, `{"target":"PAYER_B_POLICY"}`)
	if response.Code != http.StatusBadRequest || demo.calls != 1 {
		t.Fatalf("status=%d calls=%d response=%s", response.Code, demo.calls, response.Body.String())
	}
}
