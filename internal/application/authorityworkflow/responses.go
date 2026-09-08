package authorityworkflow

import (
	"context"
	"errors"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/jackc/pgx/v5"
)

func (s *Service) AdvanceOrganizationResponse(ctx context.Context, in AdvanceResponseInput) (ResponseVersionView, error) {
	if in.ActorOrganizationID != in.OrganizationID {
		return ResponseVersionView{}, ErrForbidden
	}
	stream, err := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(in.EventID), authorityledger.OrganizationID(in.OrganizationID))
	if err != nil {
		return ResponseVersionView{}, mapMissing(err)
	}
	latest, err := s.repo.LatestResponse(ctx, stream.ID)
	if err != nil {
		return ResponseVersionView{}, mapMissing(err)
	}
	next := map[string]string{"RECEIVED": "UNDER_REVIEW", "UNDER_REVIEW": "DECIDED", "DECIDED": "ACTION_COMPLETED"}[latest.State]
	if next == "" || in.TargetState != next {
		return ResponseVersionView{}, ErrConflict
	}
	if in.VersionID == "" || in.OperationID == "" || len(in.VersionID) > 32 {
		return ResponseVersionView{}, ErrInvalid
	}
	v := authorityledger.ResponseVersion{ID: authorityledger.ResponseVersionID(in.VersionID), ResponseID: latest.ResponseID, EventID: latest.EventID, OrganizationID: latest.OrganizationID, PreviousID: &latest.ID, State: next, ReceiptUnixNano: latest.ReceiptUnixNano, CanonicalVersion: 1}
	cv := canonical.ResponseV1{ResponseID: string(v.ResponseID), ResponseVersionID: string(v.ID), PreviousResponseVersionID: stringPtr(string(latest.ID)), EventID: string(v.EventID), OrganizationID: string(v.OrganizationID), ResponseState: map[string]canonical.ResponseState{"UNDER_REVIEW": 2, "DECIDED": 3, "ACTION_COMPLETED": 4}[next], ReceiptTimestamp: time.Unix(0, v.ReceiptUnixNano).UTC()}
	switch next {
	case "UNDER_REVIEW":
		if in.PolicyID == "" || in.PolicyVersionID == "" || in.PolicyText == "" || in.OccurredAt.IsZero() {
			return ResponseVersionView{}, ErrInvalid
		}
		pvalue := canonical.PolicyV1{PolicyID: in.PolicyID, OrganizationID: in.OrganizationID, PolicyVersion: in.PolicyVersionID, EffectiveFrom: in.OccurredAt, PolicyText: in.PolicyText}
		h, e := canonical.PolicyCommitment(pvalue)
		if e != nil {
			return ResponseVersionView{}, ErrInvalid
		}
		var c authorityledger.Commitment
		copy(c[:], h[:])
		p := authorityledger.Policy{ID: in.PolicyID, VersionID: in.PolicyVersionID, OrganizationID: v.OrganizationID, EffectiveFromUnixNano: in.OccurredAt.UnixNano(), Text: in.PolicyText, Commitment: c, CanonicalVersion: 1}
		if e = s.repo.CreatePolicy(ctx, p); e != nil {
			return ResponseVersionView{}, e
		}
		v.PolicyVersionID = &p.VersionID
		x := canonical.Commitment(c)
		cv.PolicyVersionHash = &x
	case "DECIDED":
		if latest.PolicyVersionID == nil || in.DecisionID == "" || in.OccurredAt.IsZero() {
			return ResponseVersionView{}, ErrInvalid
		}
		p, e := s.repo.GetPolicy(ctx, *latest.PolicyVersionID)
		if e != nil {
			return ResponseVersionView{}, e
		}
		v.PolicyVersionID = latest.PolicyVersionID
		ph := canonical.Commitment(p.Commitment)
		cv.PolicyVersionHash = &ph
		dvalue := canonical.DecisionV1{DecisionID: in.DecisionID, EventID: in.EventID, OrganizationID: in.OrganizationID, PolicyVersionHash: ph, DecisionCode: in.DecisionCode, DecisionExplanation: in.DecisionExplanation, DecidedAt: in.OccurredAt, ReviewerReference: in.ReviewerReference}
		h, e := canonical.DecisionCommitment(dvalue)
		if e != nil {
			return ResponseVersionView{}, ErrInvalid
		}
		var c authorityledger.Commitment
		copy(c[:], h[:])
		d := authorityledger.Decision{ID: in.DecisionID, EventID: v.EventID, OrganizationID: v.OrganizationID, PolicyVersionID: *latest.PolicyVersionID, Code: in.DecisionCode, Explanation: in.DecisionExplanation, ReviewerReference: in.ReviewerReference, DecidedUnixNano: in.OccurredAt.UnixNano(), Commitment: c, CanonicalVersion: 1}
		if e = s.repo.CreateDecision(ctx, d); e != nil {
			return ResponseVersionView{}, e
		}
		v.DecisionID = &d.ID
		dh := canonical.Commitment(c)
		cv.DecisionHash = &dh
	case "ACTION_COMPLETED":
		if latest.PolicyVersionID == nil || latest.DecisionID == nil || in.ActionID == "" || in.OccurredAt.IsZero() {
			return ResponseVersionView{}, ErrInvalid
		}
		p, e := s.repo.GetPolicy(ctx, *latest.PolicyVersionID)
		if e != nil {
			return ResponseVersionView{}, e
		}
		d, e := s.repo.GetDecision(ctx, *latest.DecisionID)
		if e != nil {
			return ResponseVersionView{}, e
		}
		v.PolicyVersionID = latest.PolicyVersionID
		v.DecisionID = latest.DecisionID
		ph := canonical.Commitment(p.Commitment)
		dh := canonical.Commitment(d.Commitment)
		cv.PolicyVersionHash = &ph
		cv.DecisionHash = &dh
		support := canonical.Hash(in.SupportingEvidence)
		avalue := canonical.ActionV1{ActionID: in.ActionID, EventID: in.EventID, OrganizationID: in.OrganizationID, DecisionHash: dh, ActionCode: in.ActionCode, ActionExplanation: in.ActionExplanation, SupportingEvidenceHash: &support, CompletedAt: in.OccurredAt, OperatorReference: in.OperatorReference}
		h, e := canonical.ActionCommitment(avalue)
		if e != nil {
			return ResponseVersionView{}, ErrInvalid
		}
		var c authorityledger.Commitment
		copy(c[:], h[:])
		a := authorityledger.Action{ID: in.ActionID, EventID: v.EventID, OrganizationID: v.OrganizationID, DecisionID: *latest.DecisionID, Code: in.ActionCode, Explanation: in.ActionExplanation, OperatorReference: in.OperatorReference, CompletedUnixNano: in.OccurredAt.UnixNano(), SupportingEvidence: in.SupportingEvidence, Commitment: c, CanonicalVersion: 1}
		if e = s.repo.CreateAction(ctx, a); e != nil {
			return ResponseVersionView{}, e
		}
		v.ActionID = &a.ID
		ah := canonical.Commitment(c)
		cv.ActionHash = &ah
	}
	h, err := canonical.ResponseCommitment(cv)
	if err != nil {
		return ResponseVersionView{}, ErrInvalid
	}
	copy(v.Commitment[:], h[:])
	sub := submissionFor(in.OperationID, submission.OperationResponseVersion, in.VersionID, "recordResponseVersion", in.OrganizationID, v.Commitment)
	if err = s.repo.CreateResponseWithPending(ctx, v, sub); err != nil {
		return ResponseVersionView{}, err
	}
	submitErr := s.submit.Submit(ctx, sub.OperationID)
	view, err := s.responseView(ctx, v)
	if err != nil {
		return ResponseVersionView{}, err
	}
	return view, submitErr
}
func stringPtr(v string) *string { return &v }
func (s *Service) GetOrganizationResponse(ctx context.Context, event, org string) (ResponseVersionView, error) {
	stream, e := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(event), authorityledger.OrganizationID(org))
	if e != nil {
		return ResponseVersionView{}, mapMissing(e)
	}
	v, e := s.repo.LatestResponse(ctx, stream.ID)
	if e != nil {
		return ResponseVersionView{}, mapMissing(e)
	}
	return s.responseView(ctx, v)
}
func (s *Service) ListOrganizationResponsesForEvent(ctx context.Context, event string) ([]ResponseVersionView, error) {
	streams, e := s.repo.ListResponseStreamsForEvent(ctx, authorityledger.AssertionID(event))
	if e != nil {
		return nil, e
	}
	out := make([]ResponseVersionView, 0, len(streams))
	for _, stream := range streams {
		v, x := s.repo.LatestResponse(ctx, stream.ID)
		if x != nil {
			return nil, x
		}
		view, x := s.responseView(ctx, v)
		if x != nil {
			return nil, x
		}
		out = append(out, view)
	}
	return out, nil
}
func (s *Service) GetResponseHistory(ctx context.Context, event, org string) ([]ResponseVersionView, error) {
	stream, e := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(event), authorityledger.OrganizationID(org))
	if e != nil {
		return nil, mapMissing(e)
	}
	items, e := s.repo.ResponseHistory(ctx, stream.ID)
	if e != nil {
		return nil, e
	}
	out := make([]ResponseVersionView, 0, len(items))
	for _, item := range items {
		v, x := s.responseView(ctx, item)
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) ReconcileOperation(ctx context.Context, id string) (submission.Outcome, error) {
	return s.reconcile.ReconcileOne(ctx, authorityledger.OperationID(id))
}
func (s *Service) ReconcileUnresolved(ctx context.Context, limit int) []submission.Result {
	return s.reconcile.ReconcileUnresolved(ctx, limit)
}
func mapMissing(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
