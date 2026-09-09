package rc2http

import (
	"context"
	"errors"
	"net/http"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/tamperdemo"
)

type TamperDemoService interface {
	Demonstrate(context.Context, tamperdemo.Target) (tamperdemo.Result, error)
}

// HandlerWithDemo installs the dangerous demo route explicitly while leaving
// the ordinary RC2 handler unchanged. The service independently fails closed.
func HandlerWithDemo(service Service, verification VerificationService, demo TamperDemoService) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rc2/demo/tamper", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Target string `json:"target"`
		}
		if !decode(w, r, &request) {
			return
		}
		value, err := demo.Demonstrate(r.Context(), tamperdemo.Target(request.Target))
		tamperDemoResponse(w, value, err)
	})
	mux.Handle("/", Handler(service, verification))
	return mux
}

func tamperDemoResponse(w http.ResponseWriter, value tamperdemo.Result, err error) {
	switch {
	case err == nil:
		write(w, http.StatusOK, value)
	case errors.Is(err, tamperdemo.ErrDemoTamperDisabled):
		write(w, http.StatusForbidden, map[string]string{"error": "demo capability disabled"})
	case errors.Is(err, tamperdemo.ErrUnsupportedTarget):
		write(w, http.StatusBadRequest, map[string]string{"error": "unsupported demo target"})
	case errors.Is(err, tamperdemo.ErrSyntheticFixtureMissing):
		write(w, http.StatusNotFound, map[string]string{"error": "synthetic fixture not found"})
	case errors.Is(err, tamperdemo.ErrPrecondition), errors.Is(err, tamperdemo.ErrSyntheticFixtureMismatch):
		write(w, http.StatusConflict, map[string]string{"error": "demo precondition failed"})
	case errors.Is(err, tamperdemo.ErrVerificationUnavailable):
		write(w, http.StatusServiceUnavailable, map[string]string{"error": "verification unavailable"})
	default:
		write(w, http.StatusInternalServerError, map[string]string{"error": "demo operation failed"})
	}
}
