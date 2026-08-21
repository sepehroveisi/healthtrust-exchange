package workflow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
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

type DiscoveryRequest struct{ NetworkPatientID string }

func (s *Service) discoveryScope(networkID, recordID, requesterOrg string) string {
	mac := hmac.New(sha256.New, s.NodeKey)
	_, _ = mac.Write([]byte("healthtrust-discovery-v1\x00" + networkID + "\x00" + recordID + "\x00" + requesterOrg))
	return "scope-" + hex.EncodeToString(mac.Sum(nil)[:16])
}

func (s *Service) DiscoverLocalRecords(ctx context.Context, networkID, requesterOrg string) (clinical.RecordDiscovery, error) {
	result := clinical.RecordDiscovery{Hospital: organizationName(s.OrganizationID), OrganizationID: s.OrganizationID, Scopes: []clinical.DiscoveredRecordScope{}}
	if networkID == "" || requesterOrg == "" || requesterOrg == s.OrganizationID {
		return result, ErrAccessDenied
	}
	store, ok := s.Store.(OperationalStore)
	if !ok {
		return result, errors.New("patient identity store unavailable")
	}
	patient, err := store.GetPatientByNetworkID(ctx, s.OrganizationID, networkID)
	if err != nil {
		return result, nil
	}
	records, err := s.Store.(interface {
		ListClinicalRecords(context.Context, string) ([]clinical.ClinicalRecord, error)
	}).ListClinicalRecords(ctx, patient.ID)
	if err != nil {
		return result, err
	}
	for i, record := range records {
		if record.CommitState != clinical.CommitCommitted {
			continue
		}
		result.Scopes = append(result.Scopes, clinical.DiscoveredRecordScope{ID: s.discoveryScope(networkID, record.ID, requesterOrg), Label: fmt.Sprintf("Previous record %d", i+1)})
	}
	result.RecordCount = len(result.Scopes)
	result.RecordsAvailable = result.RecordCount > 0
	return result, nil
}

func (s *Service) resolveDiscoveryScope(ctx context.Context, networkID, scopeID, requesterOrg string) (clinical.Patient, clinical.ClinicalRecord, error) {
	store, ok := s.Store.(OperationalStore)
	if !ok {
		return clinical.Patient{}, clinical.ClinicalRecord{}, ErrAccessDenied
	}
	patient, err := store.GetPatientByNetworkID(ctx, s.OrganizationID, networkID)
	if err != nil {
		return clinical.Patient{}, clinical.ClinicalRecord{}, ErrAccessDenied
	}
	records, err := s.Store.(interface {
		ListClinicalRecords(context.Context, string) ([]clinical.ClinicalRecord, error)
	}).ListClinicalRecords(ctx, patient.ID)
	if err != nil {
		return clinical.Patient{}, clinical.ClinicalRecord{}, ErrAccessDenied
	}
	for _, record := range records {
		if record.CommitState == clinical.CommitCommitted && hmac.Equal([]byte(scopeID), []byte(s.discoveryScope(networkID, record.ID, requesterOrg))) {
			return patient, record, nil
		}
	}
	return clinical.Patient{}, clinical.ClinicalRecord{}, ErrAccessDenied
}

func (s *Service) DiscoverExternalRecords(ctx context.Context, localPatientID, sourceOrg string, now time.Time) (clinical.RecordDiscovery, error) {
	store, ok := s.Store.(OperationalStore)
	if !ok {
		return clinical.RecordDiscovery{}, errors.New("patient identity store unavailable")
	}
	patient, err := store.GetPatient(ctx, s.OrganizationID, localPatientID)
	if err != nil || patient.NetworkPatientID == "" {
		return clinical.RecordDiscovery{}, ErrAccessDenied
	}
	target, err := s.peerForOrganization(sourceOrg)
	if err != nil {
		return clinical.RecordDiscovery{}, err
	}
	var result clinical.RecordDiscovery
	if err := s.requestPeerJSON(ctx, target, "/internal/record-discovery", DiscoveryRequest{patient.NetworkPatientID}, now, &result); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Service) requestPeerJSON(ctx context.Context, target, path string, payload any, now time.Time, result any) error {
	raw, _ := json.Marshal(payload)
	base, ok := s.PeerURLs[target]
	if !ok {
		return errors.New("hospital_unavailable")
	}
	env := security.PeerEnvelope{RequestID: fmt.Sprintf("%s-%d", s.NodeID, now.UnixNano()), SourceNodeID: s.NodeID, SourceOrganizationID: s.OrganizationID, TargetNodeID: target, HTTPMethod: http.MethodPost, RequestPath: path, BodyHash: sha256.Sum256(raw), IssuedAt: now.UTC(), ExpiresAt: now.UTC().Add(5 * time.Minute), Nonce: fmt.Sprint(now.UnixNano())}
	env = security.SignPeerEnvelope(env, s.NodeKey)
	body, _ := json.Marshal(map[string]any{"peer": env, "payload": payload})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(base, "/")+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := s.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return errors.New("hospital_unavailable")
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("peer rejected discovery: %s", strings.TrimSpace(string(data)))
	}
	return json.Unmarshal(data, result)
}

func organizationName(id string) string {
	if id == "hospital-a" {
		return "Hospital A"
	}
	if id == "hospital-b" {
		return "Hospital B"
	}
	return "Trusted hospital"
}
