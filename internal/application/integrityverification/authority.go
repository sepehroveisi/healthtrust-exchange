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

func authorityCanonical(assertion authorityledger.AuthorityAssertion, series authorityledger.EventSeries) (canonical.Commitment, error) {
	var previous, target *string
	if assertion.PreviousID != nil {
		value := string(*assertion.PreviousID)
		previous = &value
	}
	if assertion.TargetID != nil {
		value := string(*assertion.TargetID)
		target = &value
	}
	kind := map[string]canonical.AssertionKind{"ORIGINAL": canonical.AssertionKindOriginal, "CORRECTION": canonical.AssertionKindCorrection, "SUPERSESSION": canonical.AssertionKindSupersession, "REINSTATEMENT": canonical.AssertionKindReinstatement}[assertion.Kind]
	effect := map[string]canonical.AuthorityEffect{"EXCLUSION_ACTIVE": canonical.AuthorityEffectExclusionActive, "EXCLUSION_LIFTED": canonical.AuthorityEffectExclusionLifted}[assertion.Effect]
	return canonical.AuthorityEventCommitment(canonical.AuthorityEventV1{
		EventID:            string(assertion.ID),
		EventSeriesID:      string(assertion.SeriesID),
		PreviousEventID:    previous,
		TargetEventID:      target,
		AuthorityID:        string(series.AuthorityID),
		ProviderReference:  string(series.SubjectID),
		EventType:          canonical.EventTypeExclusion,
		AssertionKind:      kind,
		AuthorityEffect:    effect,
		EffectiveTime:      time.Unix(0, assertion.EffectiveUnixNano).UTC(),
		SourceDocumentHash: canonical.Hash(assertion.SourceDocument),
	})
}

func authorityLineageChecks(history []authorityledger.AuthorityAssertion, chain map[string]ledger.AuthorityAssertion, head ledger.ID) []Check {
	checks := make([]Check, 0, len(history)+1)
	if len(history) == 0 {
		return []Check{failed(CheckLineage, "authority-history", "non-empty", "empty", ClassInvalidLineage)}
	}
	for index, item := range history {
		expectedPrevious := ""
		expectedTarget := ""
		if item.PreviousID != nil {
			expectedPrevious = string(*item.PreviousID)
		}
		if item.TargetID != nil {
			expectedTarget = string(*item.TargetID)
		}
		if index == 0 && (item.Kind != "ORIGINAL" || expectedPrevious != "") {
			checks = append(checks, failed(CheckLineage, string(item.ID), "ORIGINAL root", item.Kind+":"+expectedPrevious, ClassInvalidLineage))
			continue
		}
		if index > 0 && (item.PreviousID == nil || string(*item.PreviousID) != string(history[index-1].ID)) {
			checks = append(checks, failed(CheckLineage, string(item.ID), string(history[index-1].ID), expectedPrevious, ClassInvalidLineage))
			continue
		}
		ledgerItem := chain[string(item.ID)]
		checks = append(checks,
			compare(CheckLineage, string(item.ID)+":previous", expectedPrevious, ledgerItem.PreviousEventID.String(), ClassInvalidLineage),
			compare(CheckLineage, string(item.ID)+":target", expectedTarget, ledgerItem.TargetEventID.String(), ClassInvalidLineage),
		)
	}
	checks = append(checks, compare(CheckLineage, string(history[len(history)-1].SeriesID), string(history[len(history)-1].ID), head.String(), ClassInvalidLineage))
	return checks
}

func (s *Service) VerifyAuthorityEvent(ctx context.Context, eventID string) (Result, error) {
	if err := validID(eventID); err != nil {
		return Result{}, err
	}
	assertion, err := s.repo.GetAssertion(ctx, authorityledger.AssertionID(eventID))
	if err != nil {
		return Result{}, missing(err)
	}
	series, err := s.repo.GetEventSeries(ctx, assertion.SeriesID)
	if err != nil {
		return Result{}, missing(err)
	}
	current, err := authorityCanonical(assertion, series)
	if err != nil {
		return Result{}, err
	}
	result := Result{ObjectType: "AUTHORITY_EVENT", ObjectID: eventID}
	result.Checks = append(result.Checks, compare(CheckCanonicalCommitment, eventID, canonical.Commitment(assertion.Commitment).String(), current.String(), ClassMismatch))
	intendedCheck, intendedUnavailable, err := s.intendedCommitmentCheck(ctx, operationAuthority(), eventID, current.String())
	if err != nil {
		return Result{}, err
	}
	result.Checks = append(result.Checks, intendedCheck)
	chainEvent, chainErr := s.ledger.GetAuthorityAssertion(ctx, ledger.MustID(eventID))
	unavailable := intendedUnavailable
	if chainErr != nil {
		result.Checks = append(result.Checks, indeterminate(CheckLedgerRecord, eventID, ClassUnavailable))
		unavailable = true
	} else {
		result.Checks = append(result.Checks,
			compare(CheckCanonicalCommitment, eventID, hashString(chainEvent.EventCommitment[:]), current.String(), ClassMismatch),
			compare(CheckIdentity, eventID, eventID, chainEvent.EventID.String(), ClassMismatch),
			compare(CheckIdentity, eventID+":series", string(assertion.SeriesID), chainEvent.EventSeriesID.String(), ClassMismatch),
			compare(CheckIdentity, eventID+":authority", string(series.AuthorityID), chainEvent.AuthorityID.String(), ClassMismatch),
			compare(CheckIdentity, eventID+":type", "1", fmt.Sprint(chainEvent.EventType), ClassMismatch),
			compare(CheckIdentity, eventID+":kind", fmt.Sprint(map[string]ledger.AssertionKind{"ORIGINAL": ledger.AssertionOriginal, "CORRECTION": ledger.AssertionCorrection, "SUPERSESSION": ledger.AssertionSupersession, "REINSTATEMENT": ledger.AssertionReinstatement}[assertion.Kind]), fmt.Sprint(chainEvent.AssertionKind), ClassMismatch),
			compare(CheckIdentity, eventID+":effect", fmt.Sprint(map[string]ledger.AuthorityEffect{"EXCLUSION_ACTIVE": ledger.EffectExclusionActive, "EXCLUSION_LIFTED": ledger.EffectExclusionLifted}[assertion.Effect]), fmt.Sprint(chainEvent.AuthorityEffect), ClassMismatch),
			compare(CheckLedgerRecord, eventID+":effective-time", fmt.Sprint(assertion.EffectiveUnixNano), fmt.Sprint(chainEvent.EffectiveTime), ClassMismatch),
		)
	}
	history, err := s.repo.AssertionHistory(ctx, assertion.SeriesID)
	if err != nil {
		return Result{}, err
	}
	chainHistory := make(map[string]ledger.AuthorityAssertion, len(history))
	var head ledger.ID
	if !unavailable {
		for _, item := range history {
			value, readErr := s.ledger.GetAuthorityAssertion(ctx, ledger.MustID(string(item.ID)))
			if readErr != nil {
				unavailable = true
				result.Checks = append(result.Checks, indeterminate(CheckLineage, string(item.ID), ClassUnavailable))
				break
			}
			chainHistory[string(item.ID)] = value
		}
		if !unavailable {
			head, err = s.ledger.GetCurrentAuthorityHead(ctx, ledger.MustID(string(assertion.SeriesID)))
			if err != nil {
				unavailable = true
				result.Checks = append(result.Checks, indeterminate(CheckLineage, string(assertion.SeriesID), ClassUnavailable))
			} else {
				result.Checks = append(result.Checks, authorityLineageChecks(history, chainHistory, head)...)
			}
		}
	}
	txCheck, txUnavailable, err := s.transactionCheck(ctx, operationAuthority(), eventID)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return Result{}, err
	}
	if err == nil {
		result.Checks = append(result.Checks, txCheck)
		unavailable = unavailable || txUnavailable
	}
	result.Status = statusOf(result.Checks)
	if unavailable && result.Status != StatusFailed {
		return result, ErrUnavailable
	}
	return result, nil
}
