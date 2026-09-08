package rc2http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/application/authorityworkflow"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
)

const maxBody = 1 << 20

type Service interface {
	RegisterAuthorityEvent(context.Context, authorityworkflow.AuthorityEventInput) (authorityworkflow.EventView, error)
	GetAuthorityEvent(context.Context, string) (authorityworkflow.EventView, error)
	ListAuthorityEvents(context.Context) ([]authorityworkflow.EventView, error)
	StartOrganizationResponse(context.Context, authorityworkflow.StartResponseInput) (authorityworkflow.ResponseVersionView, error)
	AdvanceOrganizationResponse(context.Context, authorityworkflow.AdvanceResponseInput) (authorityworkflow.ResponseVersionView, error)
	GetOrganizationResponse(context.Context, string, string) (authorityworkflow.ResponseVersionView, error)
	ListOrganizationResponsesForEvent(context.Context, string) ([]authorityworkflow.ResponseVersionView, error)
	GetResponseHistory(context.Context, string, string) ([]authorityworkflow.ResponseVersionView, error)
	ReconcileOperation(context.Context, string) (submission.Outcome, error)
	ReconcileUnresolved(context.Context, int) []submission.Result
}

func Handler(service Service) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/rc2/authority-events", func(w http.ResponseWriter, r *http.Request) {
		var in authorityworkflow.AuthorityEventInput
		if !decode(w, r, &in) {
			return
		}
		v, e := service.RegisterAuthorityEvent(r.Context(), in)
		respond(w, http.StatusCreated, v, e)
	})
	mux.HandleFunc("GET /api/rc2/authority-events", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.ListAuthorityEvents(r.Context())
		respond(w, http.StatusOK, v, e)
	})
	mux.HandleFunc("GET /api/rc2/authority-events/{event}", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.GetAuthorityEvent(r.Context(), r.PathValue("event"))
		respond(w, http.StatusOK, v, e)
	})
	mux.HandleFunc("POST /api/rc2/authority-events/{event}/responses/{org}", func(w http.ResponseWriter, r *http.Request) {
		var in authorityworkflow.StartResponseInput
		if !decode(w, r, &in) {
			return
		}
		in.EventID = r.PathValue("event")
		in.OrganizationID = r.PathValue("org")
		in.ActorOrganizationID = r.Header.Get("X-Organization-ID")
		v, e := service.StartOrganizationResponse(r.Context(), in)
		respond(w, http.StatusCreated, v, e)
	})
	mux.HandleFunc("POST /api/rc2/authority-events/{event}/responses/{org}/advance", func(w http.ResponseWriter, r *http.Request) {
		var in authorityworkflow.AdvanceResponseInput
		if !decode(w, r, &in) {
			return
		}
		in.EventID = r.PathValue("event")
		in.OrganizationID = r.PathValue("org")
		in.ActorOrganizationID = r.Header.Get("X-Organization-ID")
		v, e := service.AdvanceOrganizationResponse(r.Context(), in)
		respond(w, http.StatusCreated, v, e)
	})
	mux.HandleFunc("GET /api/rc2/authority-events/{event}/responses", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.ListOrganizationResponsesForEvent(r.Context(), r.PathValue("event"))
		respond(w, http.StatusOK, v, e)
	})
	mux.HandleFunc("GET /api/rc2/authority-events/{event}/responses/{org}", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.GetOrganizationResponse(r.Context(), r.PathValue("event"), r.PathValue("org"))
		respond(w, http.StatusOK, v, e)
	})
	mux.HandleFunc("GET /api/rc2/authority-events/{event}/responses/{org}/history", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.GetResponseHistory(r.Context(), r.PathValue("event"), r.PathValue("org"))
		respond(w, http.StatusOK, v, e)
	})
	mux.HandleFunc("POST /api/rc2/reconciliation/{operation}", func(w http.ResponseWriter, r *http.Request) {
		v, e := service.ReconcileOperation(r.Context(), r.PathValue("operation"))
		respond(w, http.StatusOK, map[string]any{"outcome": v}, e)
	})
	mux.HandleFunc("POST /api/rc2/reconciliation", func(w http.ResponseWriter, r *http.Request) {
		limit, e := strconv.Atoi(r.URL.Query().Get("limit"))
		if e != nil || limit <= 0 {
			write(w, http.StatusBadRequest, map[string]string{"error": "invalid limit"})
			return
		}
		write(w, http.StatusOK, service.ReconcileUnresolved(r.Context(), limit))
	})
	return mux
}
func decode(w http.ResponseWriter, r *http.Request, target any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(target); e != nil {
		write(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return false
	}
	return true
}
func respond(w http.ResponseWriter, success int, value any, err error) {
	if err == nil {
		write(w, success, value)
		return
	}
	code := http.StatusInternalServerError
	message := "operation failed"
	switch {
	case errors.Is(err, authorityworkflow.ErrInvalid), errors.Is(err, submission.ErrUnsupportedIdentifier):
		code = http.StatusBadRequest
		message = "invalid request"
	case errors.Is(err, authorityworkflow.ErrNotFound):
		code = http.StatusNotFound
		message = "not found"
	case errors.Is(err, authorityworkflow.ErrConflict), errors.Is(err, authorityworkflow.ErrForbidden), errors.Is(err, submission.ErrOnChainConflict), errors.Is(err, submission.ErrRetryNotPermitted):
		code = http.StatusConflict
		message = "operation conflict"
	case errors.Is(err, submission.ErrChainUnavailable), errors.Is(err, ledger.ErrTransport), errors.Is(err, ledger.ErrReceiptTimeout), errors.Is(err, submission.ErrReceiptPending):
		code = http.StatusServiceUnavailable
		message = "ledger unavailable"
	}
	write(w, code, map[string]string{"error": message})
}
func write(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
