package authorityledger

import (
	"context"
	"errors"
)

func (s *Store) validateResponseReferences(ctx context.Context, v ResponseVersion) error {
	if v.PolicyVersionID != nil {
		var organization OrganizationID
		if err := s.pool.QueryRow(ctx, `SELECT organization_id FROM authority_ledger.policies WHERE version_id=$1`, *v.PolicyVersionID).Scan(&organization); err != nil {
			return err
		}
		if organization != v.OrganizationID {
			return errors.New("response policy organization mismatch")
		}
	}
	if v.DecisionID != nil {
		var event AssertionID
		var organization OrganizationID
		if err := s.pool.QueryRow(ctx, `SELECT event_id,organization_id FROM authority_ledger.decisions WHERE id=$1`, *v.DecisionID).Scan(&event, &organization); err != nil {
			return err
		}
		if event != v.EventID || organization != v.OrganizationID {
			return errors.New("response decision context mismatch")
		}
	}
	if v.ActionID != nil {
		var event AssertionID
		var organization OrganizationID
		var decision string
		if err := s.pool.QueryRow(ctx, `SELECT event_id,organization_id,decision_id FROM authority_ledger.actions WHERE id=$1`, *v.ActionID).Scan(&event, &organization, &decision); err != nil {
			return err
		}
		if event != v.EventID || organization != v.OrganizationID || v.DecisionID == nil || decision != *v.DecisionID {
			return errors.New("response action context mismatch")
		}
	}
	return nil
}
