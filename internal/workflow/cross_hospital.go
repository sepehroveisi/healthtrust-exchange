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

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/blockchain"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/security"
)

type ReferralDelivery struct {
	Referral      clinical.Referral
	TransactionID string
}
type AccessStatusDelivery struct {
	RequestID, ReferralID, RecordID string
	Status                          clinical.AccessStatus
}

func referralHash(v clinical.Referral) [32]byte {
	return clinical.HashEvent(v.ID, v.PatientID, v.NetworkPatientID, v.RecordID, v.SourceOrganizationID, v.DestinationOrganizationID, v.CreatedByActorID, string(clinical.ReferralCreated), v.CreatedAt.UTC().Format(time.RFC3339Nano))
}

func (s *Service) peerForOrganization(org string) (string, error) {
	for id, p := range s.TrustedPeers {
		if p.OrganizationID == org {
			return id, nil
		}
	}
	return "", errors.New("trusted destination hospital not configured")
}

func (s *Service) sendPeerJSON(ctx context.Context, target, path string, payload any, now time.Time) error {
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
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return fmt.Errorf("peer rejected workflow message: %s", strings.TrimSpace(string(data)))
	}
	return nil
}

func (s *Service) DeliverReferral(ctx context.Context, v clinical.Referral, now time.Time) error {
	target, err := s.peerForOrganization(v.DestinationOrganizationID)
	if err != nil {
		return err
	}
	return s.sendPeerJSON(ctx, target, "/internal/referrals", ReferralDelivery{v, "referral-" + v.ID}, now)
}

func (s *Service) ReceiveReferral(ctx context.Context, d ReferralDelivery, now time.Time) error {
	v := d.Referral
	if v.DestinationOrganizationID != s.OrganizationID || v.SourceOrganizationID == s.OrganizationID || v.ID == "" || v.PatientID == "" || v.NetworkPatientID == "" || v.RecordID == "" || d.TransactionID != "referral-"+v.ID {
		return ErrAccessDenied
	}
	actor, err := s.Registry.GetActor(v.CreatedByActorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != v.SourceOrganizationID {
		return ErrAccessDenied
	}
	if !s.transactionMatches(d.TransactionID, blockchain.ReferralCreated, v.ID, referralHash(v)) {
		return errors.New("referral blockchain commitment not synchronized")
	}
	store, ok := s.Store.(CrossHospitalStore)
	if !ok {
		return errors.New("cross-hospital persistence unavailable")
	}
	v.Status = clinical.ReferralReceived
	return store.SaveIncomingReferral(ctx, clinical.IncomingReferral{Referral: v, TransactionID: d.TransactionID, ReceivedAt: now.UTC()})
}

func (s *Service) CreateOutgoingAccessRequest(ctx context.Context, v clinical.AccessRequest, now time.Time) error {
	store, ok := s.Store.(CrossHospitalStore)
	if !ok {
		return errors.New("cross-hospital persistence unavailable")
	}
	var sourceOrg string
	if v.ReferralID != "" {
		ref, err := store.GetIncomingReferral(ctx, v.ReferralID)
		if err != nil {
			return errors.New("incoming referral not found")
		}
		r := ref.Referral
		ops, ok := s.Store.(OperationalStore)
		if !ok {
			return ErrAccessDenied
		}
		patient, patientErr := ops.GetPatient(ctx, s.OrganizationID, v.PatientID)
		if patientErr != nil || patient.NetworkPatientID == "" || patient.NetworkPatientID != r.NetworkPatientID || v.RecordID != r.RecordID || v.RequesterOrganizationID != r.DestinationOrganizationID || s.OrganizationID != r.DestinationOrganizationID {
			return ErrAccessDenied
		}
		v.NetworkPatientID = r.NetworkPatientID
		sourceOrg = r.SourceOrganizationID
	} else {
		ops, ok := s.Store.(OperationalStore)
		if !ok {
			return ErrAccessDenied
		}
		patient, err := ops.GetPatient(ctx, s.OrganizationID, v.PatientID)
		if err != nil || v.NetworkPatientID == "" || patient.NetworkPatientID != v.NetworkPatientID || v.DiscoveryScopeID == "" || v.RecordID != "" || v.SourceOrganizationID == "" || v.Purpose == "" {
			return ErrAccessDenied
		}
		sourceOrg = v.SourceOrganizationID
		if _, err := s.peerForOrganization(sourceOrg); err != nil {
			return ErrAccessDenied
		}
	}
	actor, err := s.Registry.GetActor(v.RequesterActorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != v.RequesterOrganizationID {
		return ErrAccessDenied
	}
	v.Status = clinical.AccessPending
	v.CreatedAt = now.UTC()
	v.UpdatedAt = v.CreatedAt
	out := clinical.OutgoingAccessRequest{Request: v, SourceOrganizationID: s.OrganizationID, DestinationOrganizationID: sourceOrg}
	if err := store.SaveOutgoingAccessRequest(ctx, out); err != nil {
		return err
	}
	target, err := s.peerForOrganization(sourceOrg)
	if err != nil {
		return err
	}
	if err = s.sendPeerJSON(ctx, target, "/internal/access-requests", out, now); err != nil {
		return err
	}
	if v.ReferralID != "" {
		_ = store.SetIncomingReferralStatus(ctx, v.ReferralID, clinical.ReferralAccessRequested)
	}
	return nil
}

func (s *Service) ReceiveAccessRequest(ctx context.Context, out clinical.OutgoingAccessRequest) error {
	store, ok := s.Store.(CrossHospitalStore)
	if !ok {
		return errors.New("cross-hospital persistence unavailable")
	}
	v := out.Request
	if out.DestinationOrganizationID != s.OrganizationID || out.SourceOrganizationID != v.RequesterOrganizationID {
		return ErrAccessDenied
	}
	if v.ReferralID != "" {
		ref, err := store.GetReferral(ctx, v.ReferralID)
		if err != nil {
			return ErrAccessDenied
		}
		if ref.NetworkPatientID == "" || ref.NetworkPatientID != v.NetworkPatientID || ref.RecordID != v.RecordID || ref.SourceOrganizationID != out.DestinationOrganizationID || ref.DestinationOrganizationID != out.SourceOrganizationID || v.RequesterOrganizationID != ref.DestinationOrganizationID || s.OrganizationID != ref.SourceOrganizationID {
			return ErrAccessDenied
		}
		v.PatientID = ref.PatientID
	} else {
		patient, record, err := s.resolveDiscoveryScope(ctx, v.NetworkPatientID, v.DiscoveryScopeID, v.RequesterOrganizationID)
		if err != nil || v.RecordID != "" || v.SourceOrganizationID != s.OrganizationID {
			return ErrAccessDenied
		}
		v.PatientID = patient.ID
		v.RecordID = record.ID
	}
	actor, err := s.Registry.GetActor(v.RequesterActorID)
	if err != nil || actor.Role != identity.RoleDoctor || actor.OrganizationID != v.RequesterOrganizationID {
		return ErrAccessDenied
	}
	if _, err := s.Store.GetClinicalRecord(ctx, v.RecordID); err != nil {
		return ErrAccessDenied
	}
	if err = s.RequestAccess(ctx, v); err != nil {
		return err
	}
	if v.ReferralID != "" {
		return store.SetReferralStatus(ctx, v.ReferralID, clinical.ReferralAccessRequested)
	}
	return nil
}

func (s *Service) transactionMatches(id string, typ blockchain.TransactionType, resource string, hash [32]byte) bool {
	n, ok := s.Node.(interface{ Blocks() []blockchain.Block })
	if !ok {
		return false
	}
	for _, b := range n.Blocks() {
		for _, tx := range b.Transactions {
			if tx.ID == id && tx.Type == typ && tx.ResourceID == resource && bytes.Equal(tx.PayloadHash, hash[:]) {
				return true
			}
		}
	}
	return false
}

func (s *Service) VerifyRecordAgainstBlockchain(record clinical.ClinicalRecord) bool {
	return s.transactionMatches("record-commit-"+record.ID, blockchain.RecordCommitted, record.ID, clinical.HashRecord(record))
}
func (s *Service) DeliverAccessStatus(ctx context.Context, v AccessStatusDelivery, destinationOrg string, now time.Time) error {
	target, err := s.peerForOrganization(destinationOrg)
	if err != nil {
		return err
	}
	return s.sendPeerJSON(ctx, target, "/internal/access-request-status", v, now)
}
func (s *Service) ReceiveAccessStatus(ctx context.Context, v AccessStatusDelivery, sourceOrg string) error {
	store, ok := s.Store.(CrossHospitalStore)
	if !ok {
		return errors.New("cross-hospital persistence unavailable")
	}
	out, err := store.GetOutgoingAccessRequest(ctx, v.RequestID)
	if err != nil || out.Request.ReferralID != v.ReferralID || out.DestinationOrganizationID != sourceOrg {
		return ErrAccessDenied
	}
	if v.RecordID != "" {
		if out.Request.RecordID != "" && out.Request.RecordID != v.RecordID {
			return ErrAccessDenied
		}
		if err := store.SetOutgoingAccessRequestResolution(ctx, v.RequestID, v.RecordID); err != nil {
			return err
		}
	}
	return store.SetOutgoingAccessRequestStatus(ctx, v.RequestID, v.Status)
}
