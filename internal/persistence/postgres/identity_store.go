package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/healthtrust-exchange/healthtrust-exchange/internal/identity"
	"github.com/jackc/pgx/v5"
)

func (s *Store) SaveOrganization(ctx context.Context, organization identity.Organization) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO organizations (id,name,status) VALUES ($1,$2,$3) ON CONFLICT (id) DO NOTHING`, organization.ID, organization.Name, organization.Status)
	if err != nil {
		return fmt.Errorf("save organization: %w", err)
	}
	return nil
}
func (s *Store) GetOrganization(ctx context.Context, id string) (identity.Organization, error) {
	var value identity.Organization
	err := s.pool.QueryRow(ctx, `SELECT id,name,status FROM organizations WHERE id=$1`, id).Scan(&value.ID, &value.Name, &value.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Organization{}, identity.ErrOrganizationNotFound
	}
	if err != nil {
		return identity.Organization{}, fmt.Errorf("get organization: %w", err)
	}
	return value, nil
}
func (s *Store) ListOrganizations(ctx context.Context) ([]identity.Organization, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,name,status FROM organizations ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list organizations: %w", err)
	}
	defer rows.Close()
	var result []identity.Organization
	for rows.Next() {
		var value identity.Organization
		if err := rows.Scan(&value.ID, &value.Name, &value.Status); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
func (s *Store) SaveActor(ctx context.Context, actor identity.Actor) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO actors (id,organization_id,role,status,public_key) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (id) DO NOTHING`, actor.ID, actor.OrganizationID, actor.Role, actor.Status, actor.PublicKey)
	if err != nil {
		return fmt.Errorf("save actor: %w", err)
	}
	return nil
}
func (s *Store) GetActor(ctx context.Context, id string) (identity.Actor, error) {
	var value identity.Actor
	err := s.pool.QueryRow(ctx, `SELECT id,organization_id,role,status,public_key FROM actors WHERE id=$1`, id).Scan(&value.ID, &value.OrganizationID, &value.Role, &value.Status, &value.PublicKey)
	if errors.Is(err, pgx.ErrNoRows) {
		return identity.Actor{}, identity.ErrActorNotFound
	}
	if err != nil {
		return identity.Actor{}, fmt.Errorf("get actor: %w", err)
	}
	return value, nil
}
func (s *Store) ListActors(ctx context.Context) ([]identity.Actor, error) {
	rows, err := s.pool.Query(ctx, `SELECT id,organization_id,role,status,public_key FROM actors ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list actors: %w", err)
	}
	defer rows.Close()
	var result []identity.Actor
	for rows.Next() {
		var value identity.Actor
		if err := rows.Scan(&value.ID, &value.OrganizationID, &value.Role, &value.Status, &value.PublicKey); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}
