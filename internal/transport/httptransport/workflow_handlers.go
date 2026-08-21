package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/workflow"
	"net/http"
	"time"
)

func HandlerWithWorkflow(base http.Handler, service *workflow.Service) http.Handler {
	mux := http.NewServeMux()
	patientQuery := func(r *http.Request) string {
		if id := r.URL.Query().Get("patientId"); id != "" {
			return id
		}
		networkID := r.URL.Query().Get("networkPatientId")
		if networkID == "" {
			return ""
		}
		if store, ok := service.Store.(workflow.OperationalStore); ok {
			if patient, err := store.GetPatientByNetworkID(r.Context(), service.OrganizationID, networkID); err == nil {
				return patient.ID
			}
		}
		return "__not_found__"
	}
	mux.Handle("/", base)
	mux.HandleFunc("POST /patients", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.Patient
		if !decodeJSON(w, r, &v) {
			return
		}
		created, err := service.RegisterPatient(r.Context(), v)
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /patients", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.OperationalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListPatients(r.Context(), service.OrganizationID, r.URL.Query().Get("q"))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /patients/{patientID}", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.OperationalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.GetPatient(r.Context(), service.OrganizationID, r.PathValue("patientID"))
		if err != nil {
			writeError(w, 404, "patient_not_found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /visits/check-in", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.Visit
		if !decodeJSON(w, r, &v) {
			return
		}
		created, err := service.CheckIn(r.Context(), v)
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /visits", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.OperationalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListVisits(r.Context(), service.OrganizationID, r.URL.Query().Get("doctorId"))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /visits/{visitID}", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.OperationalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.GetVisit(r.Context(), r.PathValue("visitID"))
		if err != nil || v.OrganizationID != service.OrganizationID {
			writeError(w, 404, "visit_not_found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /visits/{visitID}/start", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ DoctorID string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := service.StartVisit(r.Context(), r.PathValue("visitID"), body.DoctorID, time.Now().UTC()); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"started": true})
	})
	mux.HandleFunc("PUT /visits/{visitID}/draft", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DoctorID string
			Draft    clinical.Visit
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := service.SaveVisitDraft(r.Context(), r.PathValue("visitID"), body.DoctorID, body.Draft); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"saved": true})
	})
	mux.HandleFunc("POST /visits/{visitID}/complete", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			DoctorID, RecordID string
			Draft              clinical.Visit
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := service.SaveVisitDraft(r.Context(), r.PathValue("visitID"), body.DoctorID, body.Draft); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		record, err := service.CompleteVisit(r.Context(), r.PathValue("visitID"), body.DoctorID, body.RecordID, time.Now().UTC())
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, record)
	})
	mux.HandleFunc("POST /clinical-records", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.ClinicalRecord
		if !decodeJSON(w, r, &v) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, 400, "idempotency_key_required")
			return
		}
		created, err := service.CreateClinicalRecordIdempotent(r.Context(), key, v)
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]any{"id": created.ID, "committed": created.CommitState == clinical.CommitCommitted})
	})
	mux.HandleFunc("GET /clinical-records", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(interface {
			ListClinicalRecords(context.Context, string) ([]clinical.ClinicalRecord, error)
		})
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListClinicalRecords(r.Context(), patientQuery(r))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /clinical-records/{recordID}", func(w http.ResponseWriter, r *http.Request) {
		v, err := service.Store.GetClinicalRecord(r.Context(), r.PathValue("recordID"))
		if err != nil {
			writeError(w, 404, "record_not_found")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /referrals", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.Referral
		if !decodeJSON(w, r, &v) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, 400, "idempotency_key_required")
			return
		}
		created, err := service.CreateReferralIdempotent(r.Context(), key, v)
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, created)
	})
	mux.HandleFunc("GET /referrals", func(w http.ResponseWriter, r *http.Request) {
		v, err := service.Store.ListReferrals(r.Context(), r.URL.Query().Get("patientId"))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /incoming-referrals", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.CrossHospitalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListIncomingReferrals(r.Context(), r.URL.Query().Get("patientId"))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /outgoing-access-requests", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.AccessRequest
		if !decodeJSON(w, r, &v) {
			return
		}
		if err := service.CreateOutgoingAccessRequest(r.Context(), v, time.Now().UTC()); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]bool{"forwarded": true})
	})
	mux.HandleFunc("GET /external-records/discover", func(w http.ResponseWriter, r *http.Request) {
		value, err := service.DiscoverExternalRecords(r.Context(), r.URL.Query().Get("patientId"), r.URL.Query().Get("sourceOrganizationId"), time.Now().UTC())
		if err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, value)
	})
	mux.HandleFunc("GET /outgoing-access-requests", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(workflow.CrossHospitalStore)
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListOutgoingAccessRequests(r.Context(), r.URL.Query().Get("patientId"))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /access-requests", func(w http.ResponseWriter, r *http.Request) {
		var v clinical.AccessRequest
		if !decodeJSON(w, r, &v) {
			return
		}
		if err := service.RequestAccess(r.Context(), v); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]bool{"accepted": true})
	})
	mux.HandleFunc("GET /access-requests", func(w http.ResponseWriter, r *http.Request) {
		v, err := service.Store.ListAccessRequests(r.Context(), patientQuery(r))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("POST /access-requests/{requestID}/decline", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ PatientID string }
		if !decodeJSON(w, r, &body) {
			return
		}
		if err := service.DeclineAccess(r.Context(), r.PathValue("requestID"), body.PatientID, time.Now().UTC()); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"declined": true})
	})
	mux.HandleFunc("GET /consents", func(w http.ResponseWriter, r *http.Request) {
		store, ok := service.Store.(interface {
			ListConsents(context.Context, string) ([]clinical.Consent, error)
		})
		if !ok {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		v, err := store.ListConsents(r.Context(), patientQuery(r))
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /audit", func(w http.ResponseWriter, r *http.Request) {
		n, ok := service.Node.(interface{ Blocks() []blockchain.Block })
		if !ok {
			writeError(w, 503, "audit_unavailable")
			return
		}
		type event struct {
			Transaction blockchain.Transaction `json:"transaction"`
			BlockHeight uint64                 `json:"blockHeight"`
			BlockHash   [32]byte               `json:"blockHash"`
		}
		out := make([]event, 0)
		for _, b := range n.Blocks() {
			for _, tx := range b.Transactions {
				out = append(out, event{tx, b.Height, b.Hash})
			}
		}
		writeJSON(w, 200, out)
	})
	mux.HandleFunc("POST /consents/{requestID}/grant", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			ConsentID, PatientID string
			GrantedAt            time.Time
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, 400, "idempotency_key_required")
			return
		}
		if err := service.GrantConsentIdempotent(r.Context(), key, r.PathValue("requestID"), body.ConsentID, body.PatientID, body.GrantedAt); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]bool{"granted": true})
	})
	mux.HandleFunc("POST /consents/{consentID}/revoke", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			PatientID string
			RevokedAt time.Time
		}
		if !decodeJSON(w, r, &body) {
			return
		}
		key := r.Header.Get("Idempotency-Key")
		if key == "" {
			writeError(w, 400, "idempotency_key_required")
			return
		}
		if err := service.RevokeConsentIdempotent(r.Context(), key, r.PathValue("consentID"), body.PatientID, body.RevokedAt); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"revoked": true})
	})
	mux.HandleFunc("GET /clinical-records/{recordID}/verify", func(w http.ResponseWriter, r *http.Request) {
		valid, err := service.VerifyRecord(r.Context(), r.PathValue("recordID"))
		if err != nil {
			writeError(w, 404, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"integrityValid": valid})
	})
	mux.HandleFunc("POST /shared-records/{recordID}/retrieve", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ ActorID, OrganizationID, SourceNodeID, ReferralID, AccessRequestID string }
		if !decodeJSON(w, r, &body) {
			return
		}
		record, err := service.RetrieveScopedFromPeer(r.Context(), body.SourceNodeID, r.PathValue("recordID"), body.ReferralID, body.AccessRequestID, body.ActorID, body.OrganizationID, time.Now().UTC())
		if err != nil {
			status := 422
			if errors.Is(err, workflow.ErrAccessDenied) {
				status = 403
				err = errors.New("consent_required")
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, 200, record)
	})
	mux.HandleFunc("POST /shared-records/{recordID}/verify", func(w http.ResponseWriter, r *http.Request) {
		var record clinical.ClinicalRecord
		if !decodeJSON(w, r, &record) {
			return
		}
		if record.ID != r.PathValue("recordID") {
			writeError(w, 400, "record ID mismatch")
			return
		}
		writeJSON(w, 200, map[string]bool{"integrityValid": service.VerifyRecordAgainstBlockchain(record)})
	})
	mux.HandleFunc("GET /patients/{patientID}/journey", func(w http.ResponseWriter, r *http.Request) {
		v, err := service.PatientJourney(r.Context(), r.PathValue("patientID"))
		if err != nil {
			writeError(w, 503, err.Error())
			return
		}
		writeJSON(w, 200, v)
	})
	mux.HandleFunc("GET /operations/pending", func(w http.ResponseWriter, r *http.Request) {
		records, err := service.PendingOperations(r.Context())
		if err != nil {
			writeError(w, 503, "persistence_unavailable")
			return
		}
		type item struct{ Operation, ObjectID, Status string }
		result := make([]item, 0, len(records))
		for _, v := range records {
			result = append(result, item{"RecordCommitted", v.ID, string(v.CommitState)})
		}
		writeJSON(w, 200, result)
	})
	mux.HandleFunc("POST /operations/reconcile", func(w http.ResponseWriter, r *http.Request) {
		count, err := service.Reconcile(r.Context())
		if err != nil {
			writeError(w, 503, "reconciliation_failed")
			return
		}
		writeJSON(w, 200, map[string]any{"reconciled": count})
	})
	mux.HandleFunc("POST /internal/clinical-records/{recordID}", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Peer  security.PeerEnvelope   `json:"peer"`
			Proof workflow.RequesterProof `json:"proof"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		proof := request.Proof
		if proof.RecordID != r.PathValue("recordID") {
			writeError(w, 400, "record ID mismatch")
			return
		}
		proofBody, _ := json.Marshal(proof)
		if err := service.VerifyAndConsumePeer(r.Context(), request.Peer, r.Method, r.URL.Path, proofBody, time.Now().UTC()); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		record, err := service.AuthorizeProofAndGet(r.Context(), proof, time.Now().UTC())
		if err != nil {
			status := 403
			if !errors.Is(err, workflow.ErrAccessDenied) {
				status = 422
			}
			writeError(w, status, err.Error())
			return
		}
		writeJSON(w, 200, record)
	})
	mux.HandleFunc("POST /internal/referrals", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Peer    security.PeerEnvelope     `json:"peer"`
			Payload workflow.ReferralDelivery `json:"payload"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		raw, _ := json.Marshal(request.Payload)
		if err := service.VerifyAndConsumePeer(r.Context(), request.Peer, r.Method, r.URL.Path, raw, time.Now().UTC()); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		if request.Peer.SourceOrganizationID != request.Payload.Referral.SourceOrganizationID {
			writeError(w, 403, "referral source mismatch")
			return
		}
		if err := service.ReceiveReferral(r.Context(), request.Payload, time.Now().UTC()); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]bool{"received": true})
	})
	mux.HandleFunc("POST /internal/record-discovery", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Peer    security.PeerEnvelope     `json:"peer"`
			Payload workflow.DiscoveryRequest `json:"payload"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		raw, _ := json.Marshal(request.Payload)
		if err := service.VerifyAndConsumePeer(r.Context(), request.Peer, r.Method, r.URL.Path, raw, time.Now().UTC()); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		value, err := service.DiscoverLocalRecords(r.Context(), request.Payload.NetworkPatientID, request.Peer.SourceOrganizationID)
		if err != nil {
			writeError(w, 403, "unauthorized")
			return
		}
		writeJSON(w, 200, value)
	})
	mux.HandleFunc("POST /internal/access-requests", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Peer    security.PeerEnvelope          `json:"peer"`
			Payload clinical.OutgoingAccessRequest `json:"payload"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		raw, _ := json.Marshal(request.Payload)
		if err := service.VerifyAndConsumePeer(r.Context(), request.Peer, r.Method, r.URL.Path, raw, time.Now().UTC()); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		if request.Peer.SourceOrganizationID != request.Payload.SourceOrganizationID {
			writeError(w, 403, "request source mismatch")
			return
		}
		if err := service.ReceiveAccessRequest(r.Context(), request.Payload); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 201, map[string]bool{"accepted": true})
	})
	mux.HandleFunc("POST /internal/access-request-status", func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Peer    security.PeerEnvelope         `json:"peer"`
			Payload workflow.AccessStatusDelivery `json:"payload"`
		}
		if !decodeJSON(w, r, &request) {
			return
		}
		raw, _ := json.Marshal(request.Payload)
		if err := service.VerifyAndConsumePeer(r.Context(), request.Peer, r.Method, r.URL.Path, raw, time.Now().UTC()); err != nil {
			writeError(w, 401, "unauthorized")
			return
		}
		if err := service.ReceiveAccessStatus(r.Context(), request.Payload, request.Peer.SourceOrganizationID); err != nil {
			writeError(w, 422, err.Error())
			return
		}
		writeJSON(w, 200, map[string]bool{"updated": true})
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mux.ServeHTTP(w, r)
	})
}
