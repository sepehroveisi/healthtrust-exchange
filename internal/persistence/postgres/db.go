package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ pool *pgxpool.Pool }

func Open(ctx context.Context, databaseURL string) (*Store, error) {
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return nil, fmt.Errorf("parse database URL: %w", err)
	}
	config.MaxConns = 10
	config.MinConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create database pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect database: %w", err)
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Close() { s.pool.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, schemaSQL)
	if err != nil {
		return fmt.Errorf("run schema migration: %w", err)
	}
	return nil
}

var schemaSQL = `
CREATE TABLE IF NOT EXISTS organizations (id text PRIMARY KEY, name text NOT NULL, status text NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS actors (id text PRIMARY KEY, organization_id text NOT NULL REFERENCES organizations(id), role text NOT NULL, status text NOT NULL, public_key bytea NOT NULL CHECK (octet_length(public_key) = 32), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS blocks (height bigint PRIMARY KEY CHECK (height >= 0), hash bytea NOT NULL UNIQUE CHECK (octet_length(hash) = 32), previous_hash bytea NOT NULL CHECK (octet_length(previous_hash) = 32), timestamp_unix_nano bigint NOT NULL, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS transactions (id text PRIMARY KEY, block_height bigint NOT NULL REFERENCES blocks(height) ON DELETE CASCADE, position integer NOT NULL CHECK (position >= 0), type text NOT NULL, actor_id text NOT NULL REFERENCES actors(id), organization_id text NOT NULL REFERENCES organizations(id), resource_id text NOT NULL, payload_hash bytea NOT NULL CHECK (octet_length(payload_hash) = 32), timestamp_unix_nano bigint NOT NULL, public_key bytea NOT NULL CHECK (octet_length(public_key) = 32), signature bytea NOT NULL CHECK (octet_length(signature) = 64), UNIQUE (block_height, position));
CREATE INDEX IF NOT EXISTS transactions_block_height_idx ON transactions(block_height, position);
CREATE TABLE IF NOT EXISTS clinical_records(id text PRIMARY KEY,patient_id text NOT NULL,author_doctor_id text NOT NULL,organization_id text NOT NULL,encounter_summary text NOT NULL,diagnosis text NOT NULL,prescription text NOT NULL,created_at_unix_nano bigint NOT NULL,updated_at_unix_nano bigint NOT NULL,commit_state text NOT NULL,record_hash bytea NOT NULL CHECK(octet_length(record_hash)=32));
CREATE TABLE IF NOT EXISTS referrals(id text PRIMARY KEY,patient_id text NOT NULL,source_organization_id text NOT NULL,destination_organization_id text NOT NULL,created_by_actor_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,commit_state text NOT NULL);
CREATE TABLE IF NOT EXISTS access_requests(id text PRIMARY KEY,patient_id text NOT NULL,record_id text NOT NULL,requester_actor_id text NOT NULL,requester_organization_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,updated_at_unix_nano bigint NOT NULL);
CREATE TABLE IF NOT EXISTS consents(id text PRIMARY KEY,access_request_id text NOT NULL UNIQUE,patient_id text NOT NULL,record_id text NOT NULL,granted_to_actor_id text NOT NULL,granted_to_organization_id text NOT NULL,status text NOT NULL,granted_at_unix_nano bigint NOT NULL,revoked_at_unix_nano bigint,expires_at_unix_nano bigint);
CREATE INDEX IF NOT EXISTS referrals_patient_idx ON referrals(patient_id,created_at_unix_nano);
CREATE INDEX IF NOT EXISTS access_requests_patient_idx ON access_requests(patient_id,created_at_unix_nano);
CREATE INDEX IF NOT EXISTS active_consent_lookup_idx ON consents(patient_id,record_id,granted_to_actor_id,granted_to_organization_id,status);
CREATE TABLE IF NOT EXISTS idempotency_keys(operation text NOT NULL,key text NOT NULL,actor_id text NOT NULL,request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),status text NOT NULL,response_reference text,created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,PRIMARY KEY(operation,key));
CREATE TABLE IF NOT EXISTS consumed_requester_proofs(proof_id text PRIMARY KEY,actor_id text NOT NULL,organization_id text NOT NULL,record_id text NOT NULL,consumed_at timestamptz NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS consumed_peer_requests(request_id text PRIMARY KEY,source_node_id text NOT NULL,source_organization_id text NOT NULL,consumed_at timestamptz NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS workflow_reconciliation(operation text NOT NULL,object_id text NOT NULL,retry_count integer NOT NULL DEFAULT 0,last_error text,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(operation,object_id));
CREATE INDEX IF NOT EXISTS requester_proof_expiry_idx ON consumed_requester_proofs(expires_at);
CREATE INDEX IF NOT EXISTS peer_request_expiry_idx ON consumed_peer_requests(expires_at);`

func init() {
	schemaSQL += `
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS record_id text;
ALTER TABLE referrals ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS referral_id text;
ALTER TABLE consents ADD COLUMN IF NOT EXISTS referral_id text;
CREATE TABLE IF NOT EXISTS incoming_referrals(id text PRIMARY KEY,patient_id text NOT NULL,record_id text NOT NULL,source_organization_id text NOT NULL,destination_organization_id text NOT NULL,created_by_actor_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,commit_state text NOT NULL,transaction_id text NOT NULL,received_at_unix_nano bigint NOT NULL);
ALTER TABLE incoming_referrals ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS outgoing_access_requests(id text PRIMARY KEY,referral_id text NOT NULL,patient_id text NOT NULL,record_id text NOT NULL,requester_actor_id text NOT NULL,requester_organization_id text NOT NULL,source_organization_id text NOT NULL,destination_organization_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,updated_at_unix_nano bigint NOT NULL);
CREATE INDEX IF NOT EXISTS incoming_referrals_patient_idx ON incoming_referrals(patient_id,received_at_unix_nano);
CREATE INDEX IF NOT EXISTS outgoing_access_patient_idx ON outgoing_access_requests(patient_id,created_at_unix_nano);`
	schemaSQL += `
CREATE TABLE IF NOT EXISTS patients(id text NOT NULL,organization_id text NOT NULL,full_name text NOT NULL,date_of_birth text NOT NULL,gender text NOT NULL,phone text NOT NULL,address text NOT NULL DEFAULT '',emergency_contact text NOT NULL DEFAULT '',created_at_unix_nano bigint NOT NULL,PRIMARY KEY(organization_id,id));
CREATE TABLE IF NOT EXISTS visits(id text PRIMARY KEY,patient_id text NOT NULL,organization_id text NOT NULL,doctor_id text NOT NULL,room text NOT NULL,reason text NOT NULL,source text NOT NULL,referral_id text,status text NOT NULL,checked_in_at_unix_nano bigint NOT NULL,started_at_unix_nano bigint,completed_at_unix_nano bigint,chief_complaint text NOT NULL DEFAULT '',clinical_notes text NOT NULL DEFAULT '',diagnosis text NOT NULL DEFAULT '',prescription text NOT NULL DEFAULT '',follow_up_plan text NOT NULL DEFAULT '',record_id text);
CREATE INDEX IF NOT EXISTS visits_doctor_status_idx ON visits(organization_id,doctor_id,status,checked_in_at_unix_nano);
CREATE INDEX IF NOT EXISTS visits_patient_idx ON visits(organization_id,patient_id,checked_in_at_unix_nano);`
	schemaSQL += `
ALTER TABLE patients ADD COLUMN IF NOT EXISTS network_patient_id text;
UPDATE patients SET network_patient_id=id WHERE network_patient_id IS NULL OR network_patient_id='';
ALTER TABLE patients ALTER COLUMN network_patient_id SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS patients_network_identity_idx ON patients(organization_id,network_patient_id);
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS discovery_scope_id text NOT NULL DEFAULT '';
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS source_organization_id text NOT NULL DEFAULT '';
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT '';
ALTER TABLE outgoing_access_requests ALTER COLUMN referral_id DROP NOT NULL;
ALTER TABLE outgoing_access_requests ALTER COLUMN record_id DROP NOT NULL;
ALTER TABLE outgoing_access_requests ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
ALTER TABLE outgoing_access_requests ADD COLUMN IF NOT EXISTS discovery_scope_id text NOT NULL DEFAULT '';
ALTER TABLE outgoing_access_requests ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT '';`
}
