package integrityverification

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/evidence/canonical"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/ledger"
	"github.com/healthtrust-exchange/healthtrust-exchange/internal/persistence/authorityledger"
)

type responseEvidence struct {
	response canonical.Commitment
	policy   *canonical.Commitment
	decision *canonical.Commitment
	action   *canonical.Commitment
}

func responseState(value string) canonical.ResponseState {
	return map[string]canonical.ResponseState{
		"RECEIVED": canonical.ResponseStateReceived, "UNDER_REVIEW": canonical.ResponseStateUnderReview,
		"DECIDED": canonical.ResponseStateDecided, "ACTION_COMPLETED": canonical.ResponseStateActionCompleted,
	}[value]
}

func (s *Service) responseCanonical(ctx context.Context, version authorityledger.ResponseVersion) (responseEvidence, error) {
	evidence := responseEvidence{}
	var previous *string
	if version.PreviousID != nil {
		value := string(*version.PreviousID)
		previous = &value
	}
	if version.PolicyVersionID != nil {
		policy, err := s.repo.GetPolicy(ctx, *version.PolicyVersionID)
		if err != nil {
			return evidence, missing(err)
		}
		var until *time.Time
		if policy.EffectiveUntilUnixNano != nil {
			value := time.Unix(0, *policy.EffectiveUntilUnixNano).UTC()
			until = &value
		}
		value, err := canonical.PolicyCommitment(canonical.PolicyV1{PolicyID: policy.ID, OrganizationID: string(policy.OrganizationID), PolicyVersion: policy.VersionID, EffectiveFrom: time.Unix(0, policy.EffectiveFromUnixNano).UTC(), EffectiveUntil: until, PolicyText: policy.Text})
		if err != nil {
			return evidence, err
		}
		evidence.policy = &value
	}
	if version.DecisionID != nil {
		decision, err := s.repo.GetDecision(ctx, *version.DecisionID)
		if err != nil {
			return evidence, missing(err)
		}
		if evidence.policy == nil {
			return evidence, errors.New("integrity verification: decision without policy")
		}
		value, err := canonical.DecisionCommitment(canonical.DecisionV1{DecisionID: decision.ID, EventID: string(decision.EventID), OrganizationID: string(decision.OrganizationID), PolicyVersionHash: *evidence.policy, DecisionCode: decision.Code, DecisionExplanation: decision.Explanation, DecidedAt: time.Unix(0, decision.DecidedUnixNano).UTC(), ReviewerReference: decision.ReviewerReference})
		if err != nil {
			return evidence, err
		}
		evidence.decision = &value
	}
	if version.ActionID != nil {
		action, err := s.repo.GetAction(ctx, *version.ActionID)
		if err != nil {
			return evidence, missing(err)
		}
		if evidence.decision == nil {
			return evidence, errors.New("integrity verification: action without decision")
		}
		support := canonical.Hash(action.SupportingEvidence)
		value, err := canonical.ActionCommitment(canonical.ActionV1{ActionID: action.ID, EventID: string(action.EventID), OrganizationID: string(action.OrganizationID), DecisionHash: *evidence.decision, ActionCode: action.Code, ActionExplanation: action.Explanation, SupportingEvidenceHash: &support, CompletedAt: time.Unix(0, action.CompletedUnixNano).UTC(), OperatorReference: action.OperatorReference})
		if err != nil {
			return evidence, err
		}
		evidence.action = &value
	}
	value, err := canonical.ResponseCommitment(canonical.ResponseV1{ResponseID: string(version.ResponseID), ResponseVersionID: string(version.ID), PreviousResponseVersionID: previous, EventID: string(version.EventID), OrganizationID: string(version.OrganizationID), ResponseState: responseState(version.State), ReceiptTimestamp: time.Unix(0, version.ReceiptUnixNano).UTC(), PolicyVersionHash: evidence.policy, DecisionHash: evidence.decision, ActionHash: evidence.action})
	evidence.response = value
	return evidence, err
}

func optionalHash(value *canonical.Commitment) string {
	if value == nil {
		return canonical.Commitment{}.String()
	}
	return value.String()
}

func chainHash(value ledger.Commitment) string { return hashString(value[:]) }

func responseSnapshotChecks(local authorityledger.ResponseVersion, chain ledger.ResponseVersion, evidence responseEvidence) []Check {
	id := string(local.ID)
	previous := ""
	if local.PreviousID != nil {
		previous = string(*local.PreviousID)
	}
	return []Check{
		compare(CheckIdentity, id+":version", id, chain.ResponseVersionID.String(), ClassMismatch),
		compare(CheckIdentity, id+":stream", string(local.ResponseID), chain.ResponseID.String(), ClassMismatch),
		compare(CheckIdentity, id+":event", string(local.EventID), chain.EventID.String(), ClassMismatch),
		compare(CheckIdentity, id+":organization", string(local.OrganizationID), chain.OrganizationID.String(), ClassMismatch),
		compare(CheckResponseState, id, fmt.Sprint(responseState(local.State)), fmt.Sprint(chain.State), ClassMismatch),
		compare(CheckLineage, id, previous, chain.PreviousResponseVersionID.String(), ClassInvalidLineage),
		compare(CheckLedgerRecord, id+":receipt-time", fmt.Sprint(local.ReceiptUnixNano), fmt.Sprint(chain.ReceiptTimestamp), ClassMismatch),
		compare(CheckCanonicalCommitment, id+":policy", optionalHash(evidence.policy), chainHash(chain.PolicyVersionHash), ClassMismatch),
		compare(CheckCanonicalCommitment, id+":decision", optionalHash(evidence.decision), chainHash(chain.DecisionCommitment), ClassMismatch),
		compare(CheckCanonicalCommitment, id+":action", optionalHash(evidence.action), chainHash(chain.ActionCommitment), ClassMismatch),
	}
}

func localResponseLineage(history []authorityledger.ResponseVersion) []Check {
	checks := make([]Check, 0, len(history))
	states := []string{"RECEIVED", "UNDER_REVIEW", "DECIDED", "ACTION_COMPLETED"}
	if len(history) == 0 {
		return []Check{failed(CheckLineage, "response-history", "non-empty", "empty", ClassInvalidLineage)}
	}
	root := history[0]
	for index, item := range history {
		expectedPrevious := ""
		if index > 0 {
			expectedPrevious = string(history[index-1].ID)
		}
		observedPrevious := ""
		if item.PreviousID != nil {
			observedPrevious = string(*item.PreviousID)
		}
		validContext := item.ResponseID == root.ResponseID && item.EventID == root.EventID && item.OrganizationID == root.OrganizationID
		if index >= len(states) || item.State != states[index] || observedPrevious != expectedPrevious || !validContext {
			checks = append(checks, failed(CheckLineage, string(item.ID), fmt.Sprintf("%s:%s:%s", states[min(index, len(states)-1)], expectedPrevious, root.OrganizationID), fmt.Sprintf("%s:%s:%s", item.State, observedPrevious, item.OrganizationID), ClassInvalidLineage))
		} else {
			checks = append(checks, verified(CheckLineage, string(item.ID), expectedPrevious, observedPrevious))
		}
	}
	return checks
}

func (s *Service) verifyResponseHistory(ctx context.Context, history []authorityledger.ResponseVersion) (Result, error) {
	result := Result{ObjectType: "ORGANIZATION_RESPONSE", ObjectID: string(history[len(history)-1].ResponseID)}
	result.Checks = append(result.Checks, localResponseLineage(history)...)
	unavailable := false
	for _, local := range history {
		evidence, err := s.responseCanonical(ctx, local)
		if err != nil {
			return Result{}, err
		}
		result.Checks = append(result.Checks, compare(CheckCanonicalCommitment, string(local.ID), canonical.Commitment(local.Commitment).String(), evidence.response.String(), ClassMismatch))
		intendedCheck, intendedUnavailable, intendedErr := s.intendedCommitmentCheck(ctx, operationResponse(), string(local.ID), evidence.response.String())
		if intendedErr != nil {
			return Result{}, intendedErr
		}
		result.Checks = append(result.Checks, intendedCheck)
		unavailable = unavailable || intendedUnavailable
		chain, readErr := s.ledger.GetResponseVersion(ctx, ledger.MustID(string(local.ID)))
		if readErr != nil {
			result.Checks = append(result.Checks, indeterminate(CheckLedgerRecord, string(local.ID), ClassUnavailable))
			unavailable = true
		} else {
			result.Checks = append(result.Checks, responseSnapshotChecks(local, chain, evidence)...)
		}
		txCheck, txUnavailable, txErr := s.transactionCheck(ctx, operationResponse(), string(local.ID))
		if txErr != nil {
			return Result{}, txErr
		}
		result.Checks = append(result.Checks, txCheck)
		unavailable = unavailable || txUnavailable
	}
	latest := history[len(history)-1]
	chainLatest, err := s.ledger.GetLatestResponse(ctx, ledger.MustID(string(latest.EventID)), ledger.MustID(string(latest.OrganizationID)))
	if err != nil {
		result.Checks = append(result.Checks, indeterminate(CheckLineage, string(latest.ResponseID)+":head", ClassUnavailable))
		unavailable = true
	} else {
		result.Checks = append(result.Checks, compare(CheckLineage, string(latest.ResponseID)+":head", string(latest.ID), chainLatest.ResponseVersionID.String(), ClassInvalidLineage))
	}
	result.Status = statusOf(result.Checks)
	if unavailable && result.Status != StatusFailed {
		return result, ErrUnavailable
	}
	return result, nil
}

func (s *Service) VerifyResponse(ctx context.Context, eventID, organizationID string) (Result, error) {
	if err := validID(eventID); err != nil {
		return Result{}, err
	}
	if err := validID(organizationID); err != nil {
		return Result{}, err
	}
	stream, err := s.repo.GetResponseStreamForEvent(ctx, authorityledger.AssertionID(eventID), authorityledger.OrganizationID(organizationID))
	if err != nil {
		return Result{}, missing(err)
	}
	history, err := s.repo.ResponseHistory(ctx, stream.ID)
	if err != nil {
		return Result{}, missing(err)
	}
	return s.verifyResponseHistory(ctx, history)
}

func (s *Service) VerifyResponseVersion(ctx context.Context, versionID string) (Result, error) {
	if err := validID(versionID); err != nil {
		return Result{}, err
	}
	version, err := s.repo.GetResponse(ctx, authorityledger.ResponseVersionID(versionID))
	if err != nil {
		return Result{}, missing(err)
	}
	result := Result{ObjectType: "RESPONSE_VERSION", ObjectID: versionID}
	evidence, err := s.responseCanonical(ctx, version)
	if err != nil {
		return Result{}, err
	}
	result.Checks = append(result.Checks, compare(CheckCanonicalCommitment, versionID, canonical.Commitment(version.Commitment).String(), evidence.response.String(), ClassMismatch))
	intendedCheck, intendedUnavailable, intendedErr := s.intendedCommitmentCheck(ctx, operationResponse(), versionID, evidence.response.String())
	if intendedErr != nil {
		return Result{}, intendedErr
	}
	result.Checks = append(result.Checks, intendedCheck)
	chain, readErr := s.ledger.GetResponseVersion(ctx, ledger.MustID(versionID))
	unavailable := intendedUnavailable
	if readErr != nil {
		result.Checks = append(result.Checks, indeterminate(CheckLedgerRecord, versionID, ClassUnavailable))
		unavailable = true
	} else {
		result.Checks = append(result.Checks, responseSnapshotChecks(version, chain, evidence)...)
	}
	txCheck, txUnavailable, txErr := s.transactionCheck(ctx, operationResponse(), versionID)
	if txErr != nil {
		return Result{}, txErr
	}
	result.Checks = append(result.Checks, txCheck)
	unavailable = unavailable || txUnavailable
	result.Status = statusOf(result.Checks)
	if unavailable && result.Status != StatusFailed {
		return result, ErrUnavailable
	}
	return result, nil
}
