package workflow

import (
	"context"
	"crypto/ed25519"
	"encoding/binary"
	"errors"
	"fmt"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"time"
)

type RequesterProof struct {
	ProofID, ActorID, OrganizationID, RecordID, ReferralID, AccessRequestID string
	IssuedAt, ExpiresAt                                                     time.Time
	Signature                                                               []byte
}

func proofMessage(p RequesterProof) []byte {
	result := []byte("healthtrust-record-request-v3\x00" + p.ProofID + "\x00" + p.ActorID + "\x00" + p.OrganizationID + "\x00" + p.RecordID + "\x00" + p.ReferralID + "\x00" + p.AccessRequestID + "\x00")
	var timestamp [8]byte
	binary.BigEndian.PutUint64(timestamp[:], uint64(p.IssuedAt.UTC().UnixNano()))
	result = append(result, timestamp[:]...)
	binary.BigEndian.PutUint64(timestamp[:], uint64(p.ExpiresAt.UTC().UnixNano()))
	return append(result, timestamp[:]...)
}
func SignScopedRequesterProof(actorID, organizationID, recordID, referralID, requestID string, issuedAt time.Time, key ed25519.PrivateKey) RequesterProof {
	p := RequesterProof{ProofID: fmt.Sprintf("%s-%d", actorID, issuedAt.UnixNano()), ActorID: actorID, OrganizationID: organizationID, RecordID: recordID, ReferralID: referralID, AccessRequestID: requestID, IssuedAt: issuedAt.UTC(), ExpiresAt: issuedAt.UTC().Add(5 * time.Minute)}
	p.Signature = ed25519.Sign(key, proofMessage(p))
	return p
}
func SignRequesterProof(actorID, organizationID, recordID string, issuedAt time.Time, key ed25519.PrivateKey) RequesterProof {
	proof := RequesterProof{ProofID: fmt.Sprintf("%s-%d", actorID, issuedAt.UnixNano()), ActorID: actorID, OrganizationID: organizationID, RecordID: recordID, IssuedAt: issuedAt.UTC(), ExpiresAt: issuedAt.UTC().Add(5 * time.Minute)}
	proof.Signature = ed25519.Sign(key, proofMessage(proof))
	return proof
}
func VerifyRequesterProof(proof RequesterProof, registry identity.Registry, now time.Time) error {
	actor, err := registry.GetActor(proof.ActorID)
	if err != nil {
		return ErrAccessDenied
	}
	if actor.OrganizationID != proof.OrganizationID || actor.Role != identity.RoleDoctor || actor.Status != identity.ActorActive {
		return ErrAccessDenied
	}
	if proof.ProofID == "" || now.Before(proof.IssuedAt.Add(-time.Minute)) || now.After(proof.ExpiresAt) || proof.ExpiresAt.Sub(proof.IssuedAt) > 5*time.Minute {
		return errors.New("requester proof expired")
	}
	if !ed25519.Verify(ed25519.PublicKey(actor.PublicKey), proofMessage(proof), proof.Signature) {
		return errors.New("invalid requester proof")
	}
	return nil
}
func (s *Service) AuthorizeProofAndGet(ctx context.Context, proof RequesterProof, now time.Time) (clinical.ClinicalRecord, error) {
	if err := VerifyRequesterProof(proof, s.Registry, now); err != nil {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	if proof.AccessRequestID == "" {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	req, err := s.Store.GetAccessRequest(ctx, proof.AccessRequestID)
	if err != nil || req.ReferralID != proof.ReferralID || req.RecordID != proof.RecordID || req.RequesterActorID != proof.ActorID || req.RequesterOrganizationID != proof.OrganizationID || req.Status != clinical.AccessGranted {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	store, ok := s.Store.(CrossHospitalStore)
	if !ok {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	if proof.ReferralID != "" {
		ref, err := store.GetReferral(ctx, proof.ReferralID)
		if err != nil || ref.PatientID != req.PatientID || ref.RecordID != req.RecordID || ref.DestinationOrganizationID != req.RequesterOrganizationID {
			return clinical.ClinicalRecord{}, ErrAccessDenied
		}
	} else if req.DiscoveryScopeID == "" || req.NetworkPatientID == "" || req.SourceOrganizationID != s.OrganizationID {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	consent, err := s.Store.FindActiveConsent(ctx, req.PatientID, req.RecordID, req.RequesterActorID, req.RequesterOrganizationID, now)
	if err != nil || consent.AccessRequestID != req.ID || consent.ReferralID != req.ReferralID {
		return clinical.ClinicalRecord{}, ErrAccessDenied
	}
	record, err := s.AuthorizeAndGet(ctx, proof.RecordID, proof.ActorID, proof.OrganizationID, now)
	if err != nil {
		return clinical.ClinicalRecord{}, err
	}
	consumer, ok := s.Store.(interface {
		ConsumeRequesterProof(context.Context, string, string, string, string, time.Time, time.Time) error
	})
	if !ok {
		return clinical.ClinicalRecord{}, errors.New("proof replay store unavailable")
	}
	if err := consumer.ConsumeRequesterProof(ctx, proof.ProofID, proof.ActorID, proof.OrganizationID, proof.RecordID, now, proof.ExpiresAt); err != nil {
		return clinical.ClinicalRecord{}, err
	}
	if proof.ReferralID != "" {
		_ = store.SetReferralStatus(ctx, proof.ReferralID, clinical.ReferralCompleted)
	}
	return record, nil
}
