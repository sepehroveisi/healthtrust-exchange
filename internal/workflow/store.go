package workflow

import (
	"context"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/clinical"
	"time"
)

type Store interface {
	SaveClinicalRecord(context.Context, clinical.ClinicalRecord) error
	GetClinicalRecord(context.Context, string) (clinical.ClinicalRecord, error)
	SetRecordCommitState(context.Context, string, clinical.CommitState, []byte) error
	SaveReferral(context.Context, clinical.Referral) error
	SetReferralCommitState(context.Context, string, clinical.CommitState) error
	ListReferrals(context.Context, string) ([]clinical.Referral, error)
	ListPendingReferrals(context.Context) ([]clinical.Referral, error)
	SaveAccessRequest(context.Context, clinical.AccessRequest) error
	GetAccessRequest(context.Context, string) (clinical.AccessRequest, error)
	SetAccessRequestStatus(context.Context, string, clinical.AccessStatus) error
	ListAccessRequests(context.Context, string) ([]clinical.AccessRequest, error)
	SaveConsent(context.Context, clinical.Consent) error
	GetConsent(context.Context, string) (clinical.Consent, error)
	ActivateConsent(context.Context, string) error
	BeginConsentRevocation(context.Context, string, time.Time) error
	RevokeConsent(context.Context, string, time.Time) error
	FindActiveConsent(context.Context, string, string, string, string, time.Time) (clinical.Consent, error)
	ListPendingConsents(context.Context) ([]clinical.Consent, error)
	ListPendingRevocations(context.Context) ([]clinical.Consent, error)
}
