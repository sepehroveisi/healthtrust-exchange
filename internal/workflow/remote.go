package workflow

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
)

func (s *Service) RetrieveFromPeer(ctx context.Context, targetNodeID, recordID, actorID, organizationID string, now time.Time) (clinical.ClinicalRecord, error) {
	return s.RetrieveScopedFromPeer(ctx, targetNodeID, recordID, "", "", actorID, organizationID, now)
}
func (s *Service) RetrieveScopedFromPeer(ctx context.Context, targetNodeID, recordID, referralID, requestID, actorID, organizationID string, now time.Time) (clinical.ClinicalRecord, error) {
	actorKey, ok := s.Keys[actorID]
	if !ok || len(s.NodeKey) == 0 {
		return clinical.ClinicalRecord{}, security.ErrUnauthorized
	}
	baseURL, ok := s.PeerURLs[targetNodeID]
	if !ok {
		return clinical.ClinicalRecord{}, errors.New("hospital_unavailable")
	}
	proof := SignScopedRequesterProof(actorID, organizationID, recordID, referralID, requestID, now, actorKey)
	proofBody, _ := json.Marshal(proof)
	path := "/internal/clinical-records/" + recordID
	envelope := security.PeerEnvelope{RequestID: fmt.Sprintf("%s-%d", s.NodeID, now.UnixNano()), SourceNodeID: s.NodeID, SourceOrganizationID: organizationID, TargetNodeID: targetNodeID, HTTPMethod: http.MethodPost, RequestPath: path, BodyHash: sha256.Sum256(proofBody), IssuedAt: now.UTC(), ExpiresAt: now.UTC().Add(5 * time.Minute), Nonce: fmt.Sprintf("%d", now.UnixNano())}
	envelope = security.SignPeerEnvelope(envelope, s.NodeKey)
	body, _ := json.Marshal(map[string]any{"peer": envelope, "proof": proof})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return clinical.ClinicalRecord{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return clinical.ClinicalRecord{}, errors.New("hospital_unavailable")
	}
	defer response.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var v struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(data, &v)
		if v.Error == "record access denied" || v.Error == "active consent not found" {
			return clinical.ClinicalRecord{}, ErrAccessDenied
		}
		return clinical.ClinicalRecord{}, fmt.Errorf("remote retrieval failed: %s", v.Error)
	}
	var record clinical.ClinicalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return record, err
	}
	if !s.VerifyRecordAgainstBlockchain(record) {
		return clinical.ClinicalRecord{}, ErrIntegrityMismatch
	}
	if err := s.AuditAccess(ctx, record, actorID, organizationID, now); err != nil {
		return record, err
	}
	if store, ok := s.Store.(CrossHospitalStore); ok && referralID != "" {
		_ = store.SetIncomingReferralStatus(ctx, referralID, clinical.ReferralCompleted)
	}
	return record, nil
}
