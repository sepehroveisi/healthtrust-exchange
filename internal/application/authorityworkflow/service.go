package authorityworkflow

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/submission"
	"github.com/jackc/pgx/v5"
)

var (
	ErrInvalid   = errors.New("authority workflow: invalid input")
	ErrNotFound  = errors.New("authority workflow: not found")
	ErrConflict  = errors.New("authority workflow: conflict")
	ErrForbidden = errors.New("authority workflow: organization boundary")
)

type Submitter interface {
	Submit(context.Context, authorityledger.OperationID) error
}
type Reconciler interface {
	ReconcileOne(context.Context, authorityledger.OperationID) (submission.Outcome, error)
	ReconcileUnresolved(context.Context, int) []submission.Result
}
type Repository interface {
	GetAuthority(context.Context, authorityledger.AuthorityID) (authorityledger.Authority, error)
	GetSubject(context.Context, authorityledger.SubjectID) (authorityledger.ProfessionalSubject, error)
	GetOrganization(context.Context, authorityledger.OrganizationID) (authorityledger.Organization, error)
	CreateEventSeries(context.Context, authorityledger.EventSeries) error
	GetEventSeries(context.Context, authorityledger.SeriesID) (authorityledger.EventSeries, error)
	CreateAssertionWithPending(context.Context, authorityledger.AuthorityAssertion, authorityledger.Submission) error
	GetAssertion(context.Context, authorityledger.AssertionID) (authorityledger.AuthorityAssertion, error)
	ListAssertions(context.Context) ([]authorityledger.AuthorityAssertion, error)
	AssertionHistory(context.Context, authorityledger.SeriesID) ([]authorityledger.AuthorityAssertion, error)
	CreateResponseStream(context.Context, authorityledger.ResponseStream) error
	CreateResponseStartWithPending(context.Context, authorityledger.ResponseStream, authorityledger.ResponseVersion, authorityledger.Submission) error
	GetResponseStreamForEvent(context.Context, authorityledger.AssertionID, authorityledger.OrganizationID) (authorityledger.ResponseStream, error)
	ListResponseStreamsForEvent(context.Context, authorityledger.AssertionID) ([]authorityledger.ResponseStream, error)
	CreateResponseWithPending(context.Context, authorityledger.ResponseVersion, authorityledger.Submission) error
	GetResponse(context.Context, authorityledger.ResponseVersionID) (authorityledger.ResponseVersion, error)
	LatestResponse(context.Context, authorityledger.ResponseID) (authorityledger.ResponseVersion, error)
	ResponseHistory(context.Context, authorityledger.ResponseID) ([]authorityledger.ResponseVersion, error)
	CreatePolicy(context.Context, authorityledger.Policy) error
	CreateDecision(context.Context, authorityledger.Decision) error
	CreateAction(context.Context, authorityledger.Action) error
	GetPolicy(context.Context, string) (authorityledger.Policy, error)
	GetDecision(context.Context, string) (authorityledger.Decision, error)
	GetSubmissionByDomain(context.Context, string, string) (authorityledger.Submission, error)
}
type Service struct {
	repo      Repository
	submit    Submitter
	reconcile Reconciler
}

func New(repo Repository, submit Submitter, reconcile Reconciler) *Service {
	return &Service{repo, submit, reconcile}
}

type LedgerStatus struct {
	State           string  `json:"state"`
	OperationID     string  `json:"operationId"`
	TransactionHash []byte  `json:"transactionHash,omitempty"`
	BlockNumber     *int64  `json:"blockNumber,omitempty"`
	BlockHash       []byte  `json:"blockHash,omitempty"`
	LastErrorClass  *string `json:"lastErrorClass,omitempty"`
}
type EventView struct {
	ID, SeriesID, AuthorityID, SubjectID, EventType, AssertionKind, Effect string
	EffectiveTime                                                          time.Time
	CurrentHead                                                            string
	Commitment                                                             authorityledger.Commitment
	Ledger                                                                 LedgerStatus
	CreatedAt                                                              time.Time
}
type ResponseVersionView struct {
	ID, ResponseID, EventID, OrganizationID, State string
	PreviousID                                     *authorityledger.ResponseVersionID
	PolicyVersionID, DecisionID, ActionID          *string
	Commitment                                     authorityledger.Commitment
	Ledger                                         LedgerStatus
	CreatedAt                                      time.Time
}
type AuthorityEventInput struct {
	EventID, SeriesID, AuthorityID, SubjectID, OperationID string
	EffectiveTime                                          time.Time
	SourceDocumentHash                                     canonical.Commitment
	SourceDocument                                         []byte
}
type StartResponseInput struct {
	ActorOrganizationID, OrganizationID, EventID, ResponseID, VersionID, OperationID string
	ReceiptTime                                                                      time.Time
}
type AdvanceResponseInput struct {
	ActorOrganizationID, OrganizationID, EventID, VersionID, OperationID, TargetState                                                                                   string
	PolicyID, PolicyVersionID, PolicyText, DecisionID, DecisionCode, DecisionExplanation, ReviewerReference, ActionID, ActionCode, ActionExplanation, OperatorReference string
	SupportingEvidence                                                                                                                                                  []byte
	OccurredAt                                                                                                                                                          time.Time
}

func status(v authorityledger.Submission) LedgerStatus {
	return LedgerStatus{v.State, string(v.OperationID), v.TransactionHash, v.BlockNumber, v.BlockHash, v.LastErrorClass}
}
func submissionFor(op, kind, domain, action, signer string, c authorityledger.Commitment) authorityledger.Submission {
	return authorityledger.Submission{OperationID: authorityledger.OperationID(op), OperationType: kind, DomainVersionID: domain, ContractAction: action, SignerContext: signer, IntendedCommitment: c, State: "LOCAL_PENDING"}
}
func (s *Service) eventView(ctx context.Context, a authorityledger.AuthorityAssertion) (EventView, error) {
	series, e := s.repo.GetEventSeries(ctx, a.SeriesID)
	if e != nil {
		return EventView{}, e
	}
	sub, e := s.repo.GetSubmissionByDomain(ctx, submission.OperationAuthorityAssertion, string(a.ID))
	if e != nil {
		return EventView{}, e
	}
	history, e := s.repo.AssertionHistory(ctx, a.SeriesID)
	if e != nil {
		return EventView{}, e
	}
	head := ""
	if len(history) > 0 {
		head = string(history[len(history)-1].ID)
	}
	return EventView{string(a.ID), string(a.SeriesID), string(series.AuthorityID), string(series.SubjectID), a.EventType, a.Kind, a.Effect, time.Unix(0, a.EffectiveUnixNano).UTC(), head, a.Commitment, status(sub), a.CreatedAt}, nil
}
func (s *Service) RegisterAuthorityEvent(ctx context.Context, in AuthorityEventInput) (EventView, error) {
	if in.EventID == "" || in.SeriesID == "" || in.AuthorityID == "" || in.SubjectID == "" || in.OperationID == "" || in.EffectiveTime.IsZero() {
		return EventView{}, ErrInvalid
	}
	if len(in.EventID) > 32 || len(in.SeriesID) > 32 || len(in.AuthorityID) > 32 {
		return EventView{}, ErrInvalid
	}
	if _, e := s.repo.GetAuthority(ctx, authorityledger.AuthorityID(in.AuthorityID)); e != nil {
		return EventView{}, mapMissing(e)
	}
	if _, e := s.repo.GetSubject(ctx, authorityledger.SubjectID(in.SubjectID)); e != nil {
		return EventView{}, mapMissing(e)
	}
	evidence := canonical.AuthorityEventV1{EventID: in.EventID, EventSeriesID: in.SeriesID, AuthorityID: in.AuthorityID, ProviderReference: in.SubjectID, EventType: canonical.EventTypeExclusion, AssertionKind: canonical.AssertionKindOriginal, AuthorityEffect: canonical.AuthorityEffectExclusionActive, EffectiveTime: in.EffectiveTime, SourceDocumentHash: in.SourceDocumentHash}
	hash, e := canonical.AuthorityEventCommitment(evidence)
	if e != nil {
		return EventView{}, fmt.Errorf("%w: canonical event", ErrInvalid)
	}
	var commitment authorityledger.Commitment
	copy(commitment[:], hash[:])
	if existing, e := s.repo.GetAssertion(ctx, authorityledger.AssertionID(in.EventID)); e == nil {
		if existing.Commitment != commitment {
			return EventView{}, ErrConflict
		}
		return s.eventView(ctx, existing)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return EventView{}, e
	}
	series, e := s.repo.GetEventSeries(ctx, authorityledger.SeriesID(in.SeriesID))
	if errors.Is(e, pgx.ErrNoRows) {
		if e = s.repo.CreateEventSeries(ctx, authorityledger.EventSeries{ID: authorityledger.SeriesID(in.SeriesID), SubjectID: authorityledger.SubjectID(in.SubjectID), AuthorityID: authorityledger.AuthorityID(in.AuthorityID)}); e != nil {
			return EventView{}, e
		}
	} else if e != nil {
		return EventView{}, e
	} else if series.SubjectID != authorityledger.SubjectID(in.SubjectID) || series.AuthorityID != authorityledger.AuthorityID(in.AuthorityID) {
		return EventView{}, ErrConflict
	}
	a := authorityledger.AuthorityAssertion{ID: authorityledger.AssertionID(in.EventID), SeriesID: authorityledger.SeriesID(in.SeriesID), EventType: "EXCLUSION", Kind: "ORIGINAL", Effect: "EXCLUSION_ACTIVE", EffectiveUnixNano: in.EffectiveTime.UnixNano(), SourceDocument: in.SourceDocument, Commitment: commitment, CanonicalVersion: 1}
	sub := submissionFor(in.OperationID, submission.OperationAuthorityAssertion, in.EventID, "recordAssertion", in.AuthorityID, commitment)
	if e = s.repo.CreateAssertionWithPending(ctx, a, sub); e != nil {
		return EventView{}, e
	}
	submitErr := s.submit.Submit(ctx, sub.OperationID)
	view, e := s.eventView(ctx, a)
	if e != nil {
		return EventView{}, e
	}
	return view, submitErr
}
func (s *Service) GetAuthorityEvent(ctx context.Context, id string) (EventView, error) {
	a, e := s.repo.GetAssertion(ctx, authorityledger.AssertionID(id))
	if e != nil {
		return EventView{}, mapMissing(e)
	}
	return s.eventView(ctx, a)
}
func (s *Service) ListAuthorityEvents(ctx context.Context) ([]EventView, error) {
	items, e := s.repo.ListAssertions(ctx)
	if e != nil {
		return nil, e
	}
	out := make([]EventView, 0, len(items))
	for _, item := range items {
		v, x := s.eventView(ctx, item)
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	return out, nil
}
func (s *Service) GetAuthorityHistory(ctx context.Context, series string) ([]EventView, error) {
	items, e := s.repo.AssertionHistory(ctx, authorityledger.SeriesID(series))
	if e != nil {
		return nil, e
	}
	out := make([]EventView, 0, len(items))
	for _, item := range items {
		v, x := s.eventView(ctx, item)
		if x != nil {
			return nil, x
		}
		out = append(out, v)
	}
	return out, nil
}

func (s *Service) responseView(ctx context.Context, v authorityledger.ResponseVersion) (ResponseVersionView, error) {
	sub, e := s.repo.GetSubmissionByDomain(ctx, submission.OperationResponseVersion, string(v.ID))
	if e != nil {
		return ResponseVersionView{}, e
	}
	return ResponseVersionView{string(v.ID), string(v.ResponseID), string(v.EventID), string(v.OrganizationID), v.State, v.PreviousID, v.PolicyVersionID, v.DecisionID, v.ActionID, v.Commitment, status(sub), v.CreatedAt}, nil
}
func (s *Service) StartOrganizationResponse(ctx context.Context, in StartResponseInput) (ResponseVersionView, error) {
	if in.ActorOrganizationID != in.OrganizationID {
		return ResponseVersionView{}, ErrForbidden
	}
	if in.EventID == "" || in.OrganizationID == "" || in.ResponseID == "" || in.VersionID == "" || in.OperationID == "" || in.ReceiptTime.IsZero() {
		return ResponseVersionView{}, ErrInvalid
	}
	if len(in.EventID) > 32 || len(in.OrganizationID) > 32 || len(in.ResponseID) > 32 || len(in.VersionID) > 32 {
		return ResponseVersionView{}, ErrInvalid
	}
	if _, e := s.repo.GetAssertion(ctx, authorityledger.AssertionID(in.EventID)); e != nil {
		return ResponseVersionView{}, mapMissing(e)
	}
	if _, e := s.repo.GetOrganization(ctx, authorityledger.OrganizationID(in.OrganizationID)); e != nil {
		return ResponseVersionView{}, mapMissing(e)
	}
	evidence := canonical.ResponseV1{ResponseID: in.ResponseID, ResponseVersionID: in.VersionID, EventID: in.EventID, OrganizationID: in.OrganizationID, ResponseState: canonical.ResponseStateReceived, ReceiptTimestamp: in.ReceiptTime}
	hash, e := canonical.ResponseCommitment(evidence)
	if e != nil {
		return ResponseVersionView{}, ErrInvalid
	}
	var c authorityledger.Commitment
	copy(c[:], hash[:])
	if stream, e := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(in.EventID), authorityledger.OrganizationID(in.OrganizationID)); e == nil {
		history, x := s.repo.ResponseHistory(ctx, stream.ID)
		if x != nil {
			return ResponseVersionView{}, x
		}
		if string(stream.ID) != in.ResponseID || len(history) == 0 || history[0].Commitment != c {
			return ResponseVersionView{}, ErrConflict
		}
		latest := history[len(history)-1]
		return s.responseView(ctx, latest)
	} else if !errors.Is(e, pgx.ErrNoRows) {
		return ResponseVersionView{}, e
	}
	stream := authorityledger.ResponseStream{ID: authorityledger.ResponseID(in.ResponseID), EventID: authorityledger.AssertionID(in.EventID), OrganizationID: authorityledger.OrganizationID(in.OrganizationID)}
	v := authorityledger.ResponseVersion{ID: authorityledger.ResponseVersionID(in.VersionID), ResponseID: authorityledger.ResponseID(in.ResponseID), EventID: authorityledger.AssertionID(in.EventID), OrganizationID: authorityledger.OrganizationID(in.OrganizationID), State: "RECEIVED", ReceiptUnixNano: in.ReceiptTime.UnixNano(), Commitment: c, CanonicalVersion: 1}
	sub := submissionFor(in.OperationID, submission.OperationResponseVersion, in.VersionID, "recordResponseVersion", in.OrganizationID, c)
	if e = s.repo.CreateResponseStartWithPending(ctx, stream, v, sub); e != nil {
		return ResponseVersionView{}, e
	}
	submitErr := s.submit.Submit(ctx, sub.OperationID)
	view, e := s.responseView(ctx, v)
	if e != nil {
		return ResponseVersionView{}, e
	}
	return view, submitErr
}
