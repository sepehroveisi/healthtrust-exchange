package canonical

import (
	"fmt"
	"time"
)

type EventType uint16
type AssertionKind uint16
type AuthorityEffect uint16
type ResponseState uint16

const (
	EventTypeExclusion EventType = 1

	AssertionKindOriginal      AssertionKind = 1
	AssertionKindCorrection    AssertionKind = 2
	AssertionKindSupersession  AssertionKind = 3
	AssertionKindReinstatement AssertionKind = 4

	AuthorityEffectExclusionActive AuthorityEffect = 1
	AuthorityEffectExclusionLifted AuthorityEffect = 2

	ResponseStateReceived        ResponseState = 1
	ResponseStateUnderReview     ResponseState = 2
	ResponseStateDecided         ResponseState = 3
	ResponseStateActionCompleted ResponseState = 4
)

type AuthorityEventV1 struct {
	EventID, EventSeriesID         string
	PreviousEventID, TargetEventID *string
	AuthorityID, ProviderReference string
	EventType                      EventType
	AssertionKind                  AssertionKind
	AuthorityEffect                AuthorityEffect
	EffectiveTime                  time.Time
	SourceDocumentHash             Commitment
}

type PolicyV1 struct {
	PolicyID, OrganizationID, PolicyVersion string
	EffectiveFrom                           time.Time
	EffectiveUntil                          *time.Time
	PolicyText                              string
}

type DecisionV1 struct {
	DecisionID, EventID, OrganizationID string
	PolicyVersionHash                   Commitment
	DecisionCode, DecisionExplanation   string
	DecidedAt                           time.Time
	ReviewerReference                   string
}

type ActionV1 struct {
	ActionID, EventID, OrganizationID string
	DecisionHash                      Commitment
	ActionCode, ActionExplanation     string
	SupportingEvidenceHash            *Commitment
	CompletedAt                       time.Time
	OperatorReference                 string
}

type ResponseV1 struct {
	ResponseID, ResponseVersionID string
	PreviousResponseVersionID     *string
	EventID, OrganizationID       string
	ResponseState                 ResponseState
	ReceiptTimestamp              time.Time
	PolicyVersionHash             *Commitment
	DecisionHash                  *Commitment
	ActionHash                    *Commitment
}

func encodeIdentifiers(e *encoder, values ...string) error {
	for _, value := range values {
		if err := e.identifier(value); err != nil {
			return err
		}
	}
	return nil
}

func validEventType(v EventType) bool { return v == EventTypeExclusion }
func validAssertionKind(v AssertionKind) bool {
	return v >= AssertionKindOriginal && v <= AssertionKindReinstatement
}
func validAuthorityEffect(v AuthorityEffect) bool {
	return v >= AuthorityEffectExclusionActive && v <= AuthorityEffectExclusionLifted
}
func validResponseState(v ResponseState) bool {
	return v >= ResponseStateReceived && v <= ResponseStateActionCompleted
}

func AuthorityEventCanonicalBytes(v AuthorityEventV1) ([]byte, error) {
	e := newEncoder(1)
	if err := encodeIdentifiers(e, v.EventID, v.EventSeriesID); err != nil {
		return nil, err
	}
	if err := e.nullableIdentifier(v.PreviousEventID); err != nil {
		return nil, err
	}
	if err := e.nullableIdentifier(v.TargetEventID); err != nil {
		return nil, err
	}
	if err := encodeIdentifiers(e, v.AuthorityID, v.ProviderReference); err != nil {
		return nil, err
	}
	if !validEventType(v.EventType) {
		return nil, fmt.Errorf("canonical: invalid event type %d", v.EventType)
	}
	if !validAssertionKind(v.AssertionKind) {
		return nil, fmt.Errorf("canonical: invalid assertion kind %d", v.AssertionKind)
	}
	if !validAuthorityEffect(v.AuthorityEffect) {
		return nil, fmt.Errorf("canonical: invalid authority effect %d", v.AuthorityEffect)
	}
	e.u16(uint16(v.EventType))
	e.u16(uint16(v.AssertionKind))
	e.u16(uint16(v.AuthorityEffect))
	if err := e.timestamp(v.EffectiveTime); err != nil {
		return nil, err
	}
	e.commitment(v.SourceDocumentHash)
	return e.Bytes(), nil
}

func AuthorityEventCommitment(v AuthorityEventV1) (Commitment, error) {
	b, err := AuthorityEventCanonicalBytes(v)
	if err != nil {
		return Commitment{}, err
	}
	return Hash(b), nil
}

func PolicyCanonicalBytes(v PolicyV1) ([]byte, error) {
	e := newEncoder(2)
	if err := encodeIdentifiers(e, v.PolicyID, v.OrganizationID, v.PolicyVersion); err != nil {
		return nil, err
	}
	if err := e.timestamp(v.EffectiveFrom); err != nil {
		return nil, err
	}
	if err := e.nullableTimestamp(v.EffectiveUntil); err != nil {
		return nil, err
	}
	if err := e.text(v.PolicyText); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}

func PolicyCommitment(v PolicyV1) (Commitment, error) {
	b, err := PolicyCanonicalBytes(v)
	if err != nil {
		return Commitment{}, err
	}
	return Hash(b), nil
}

func DecisionCanonicalBytes(v DecisionV1) ([]byte, error) {
	e := newEncoder(3)
	if err := encodeIdentifiers(e, v.DecisionID, v.EventID, v.OrganizationID); err != nil {
		return nil, err
	}
	e.commitment(v.PolicyVersionHash)
	if err := e.text(v.DecisionCode); err != nil {
		return nil, err
	}
	if err := e.text(v.DecisionExplanation); err != nil {
		return nil, err
	}
	if err := e.timestamp(v.DecidedAt); err != nil {
		return nil, err
	}
	if err := e.identifier(v.ReviewerReference); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}

func DecisionCommitment(v DecisionV1) (Commitment, error) {
	b, err := DecisionCanonicalBytes(v)
	if err != nil {
		return Commitment{}, err
	}
	return Hash(b), nil
}

func ActionCanonicalBytes(v ActionV1) ([]byte, error) {
	e := newEncoder(4)
	if err := encodeIdentifiers(e, v.ActionID, v.EventID, v.OrganizationID); err != nil {
		return nil, err
	}
	e.commitment(v.DecisionHash)
	if err := e.text(v.ActionCode); err != nil {
		return nil, err
	}
	if err := e.text(v.ActionExplanation); err != nil {
		return nil, err
	}
	e.nullableCommitment(v.SupportingEvidenceHash)
	if err := e.timestamp(v.CompletedAt); err != nil {
		return nil, err
	}
	if err := e.identifier(v.OperatorReference); err != nil {
		return nil, err
	}
	return e.Bytes(), nil
}

func ActionCommitment(v ActionV1) (Commitment, error) {
	b, err := ActionCanonicalBytes(v)
	if err != nil {
		return Commitment{}, err
	}
	return Hash(b), nil
}

func ResponseCanonicalBytes(v ResponseV1) ([]byte, error) {
	e := newEncoder(5)
	if err := encodeIdentifiers(e, v.ResponseID, v.ResponseVersionID); err != nil {
		return nil, err
	}
	if err := e.nullableIdentifier(v.PreviousResponseVersionID); err != nil {
		return nil, err
	}
	if err := encodeIdentifiers(e, v.EventID, v.OrganizationID); err != nil {
		return nil, err
	}
	if !validResponseState(v.ResponseState) {
		return nil, fmt.Errorf("canonical: invalid response state %d", v.ResponseState)
	}
	e.u16(uint16(v.ResponseState))
	if err := e.timestamp(v.ReceiptTimestamp); err != nil {
		return nil, err
	}
	e.nullableCommitment(v.PolicyVersionHash)
	e.nullableCommitment(v.DecisionHash)
	e.nullableCommitment(v.ActionHash)
	return e.Bytes(), nil
}

func ResponseCommitment(v ResponseV1) (Commitment, error) {
	b, err := ResponseCanonicalBytes(v)
	if err != nil {
		return Commitment{}, err
	}
	return Hash(b), nil
}
