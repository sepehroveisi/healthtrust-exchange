package authorityledger

import "context"

func (s *Store) ListAssertions(c context.Context) (out []AuthorityAssertion, err error) {
	rows, err := s.pool.Query(c, `SELECT id FROM authority_ledger.authority_assertions ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id AssertionID
		if err = rows.Scan(&id); err != nil {
			return nil, err
		}
		v, e := s.GetAssertion(c, id)
		if e != nil {
			return nil, e
		}
		out = append(out, v)
	}
	return out, rows.Err()
}
func (s *Store) GetResponseStreamForEvent(c context.Context, event AssertionID, organization OrganizationID) (v ResponseStream, err error) {
	err = s.pool.QueryRow(c, `SELECT response_id,event_id,organization_id FROM authority_ledger.response_streams WHERE event_id=$1 AND organization_id=$2`, event, organization).Scan(&v.ID, &v.EventID, &v.OrganizationID)
	return
}
func (s *Store) ListResponseStreamsForEvent(c context.Context, event AssertionID) (out []ResponseStream, err error) {
	rows, err := s.pool.Query(c, `SELECT response_id,event_id,organization_id FROM authority_ledger.response_streams WHERE event_id=$1 ORDER BY organization_id,response_id`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var v ResponseStream
		if err = rows.Scan(&v.ID, &v.EventID, &v.OrganizationID); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// CreateResponseStartWithPending atomically creates a response stream, its
// initial RECEIVED version, and the durable ledger submission intent.
func (s *Store) CreateResponseStartWithPending(c context.Context, stream ResponseStream, version ResponseVersion, pending Submission) error {
	tx, err := s.pool.Begin(c)
	if err != nil {
		return err
	}
	defer tx.Rollback(c)
	if _, err = tx.Exec(c, `INSERT INTO authority_ledger.response_streams(response_id,event_id,organization_id) VALUES($1,$2,$3)`, stream.ID, stream.EventID, stream.OrganizationID); err == nil {
		err = insertResponseTx(c, tx, version)
	}
	if err == nil {
		err = insertSubmission(c, tx, pending)
	}
	if err != nil {
		return err
	}
	return tx.Commit(c)
}
