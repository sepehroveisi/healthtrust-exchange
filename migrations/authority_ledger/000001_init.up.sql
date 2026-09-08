CREATE SCHEMA IF NOT EXISTS authority_ledger;

CREATE TABLE authority_ledger.organizations (
  id text PRIMARY KEY, type text NOT NULL CHECK (type IN ('HOSPITAL','PAYER','STAFFING_AGENCY')),
  display_name text NOT NULL, active boolean NOT NULL DEFAULT true,
  signer_address bytea NOT NULL CHECK (octet_length(signer_address)=20), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.authorities (
  id text PRIMARY KEY, type text NOT NULL CHECK (type='EXCLUSION_AUTHORITY'),
  display_name text NOT NULL, active boolean NOT NULL DEFAULT true,
  signer_address bytea NOT NULL CHECK (octet_length(signer_address)=20), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.professional_subjects (
  id text PRIMARY KEY, display_name text NOT NULL, details jsonb NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.event_series (
  id text PRIMARY KEY, subject_id text NOT NULL REFERENCES authority_ledger.professional_subjects(id),
  authority_id text NOT NULL REFERENCES authority_ledger.authorities(id), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.authority_assertions (
  id text NOT NULL, series_id text NOT NULL REFERENCES authority_ledger.event_series(id),
  previous_id text, target_id text, event_type text NOT NULL CHECK(event_type='EXCLUSION'),
  assertion_kind text NOT NULL CHECK(assertion_kind IN ('ORIGINAL','CORRECTION','SUPERSESSION','REINSTATEMENT')),
  effect text NOT NULL CHECK(effect IN ('EXCLUSION_ACTIVE','EXCLUSION_LIFTED')),
  effective_unix_nano bigint NOT NULL, source_document bytea NOT NULL,
  commitment bytea NOT NULL CHECK(octet_length(commitment)=32), canonical_version smallint NOT NULL CHECK(canonical_version=1),
  created_at timestamptz NOT NULL DEFAULT now(), PRIMARY KEY(id), UNIQUE(id,series_id),
  FOREIGN KEY(previous_id,series_id) REFERENCES authority_ledger.authority_assertions(id,series_id),
  FOREIGN KEY(target_id,series_id) REFERENCES authority_ledger.authority_assertions(id,series_id),
  CHECK ((assertion_kind='ORIGINAL' AND previous_id IS NULL AND target_id IS NULL) OR
         (assertion_kind<>'ORIGINAL' AND previous_id IS NOT NULL AND target_id IS NOT NULL)));
CREATE UNIQUE INDEX authority_assertions_one_root ON authority_ledger.authority_assertions(series_id) WHERE previous_id IS NULL;
CREATE UNIQUE INDEX authority_assertions_one_successor ON authority_ledger.authority_assertions(series_id,previous_id) WHERE previous_id IS NOT NULL;

CREATE TABLE authority_ledger.policies (
  id text NOT NULL, version_id text PRIMARY KEY, organization_id text NOT NULL REFERENCES authority_ledger.organizations(id),
  effective_from_unix_nano bigint NOT NULL, effective_until_unix_nano bigint, policy_text text NOT NULL,
  commitment bytea NOT NULL CHECK(octet_length(commitment)=32), canonical_version smallint NOT NULL CHECK(canonical_version=1),
  created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.decisions (
  id text PRIMARY KEY, event_id text NOT NULL REFERENCES authority_ledger.authority_assertions(id),
  organization_id text NOT NULL REFERENCES authority_ledger.organizations(id), policy_version_id text NOT NULL REFERENCES authority_ledger.policies(version_id),
  decision_code text NOT NULL, explanation text NOT NULL, reviewer_reference text NOT NULL, decided_unix_nano bigint NOT NULL,
  commitment bytea NOT NULL CHECK(octet_length(commitment)=32), canonical_version smallint NOT NULL CHECK(canonical_version=1), created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE authority_ledger.actions (
  id text PRIMARY KEY, event_id text NOT NULL REFERENCES authority_ledger.authority_assertions(id),
  organization_id text NOT NULL REFERENCES authority_ledger.organizations(id), decision_id text NOT NULL REFERENCES authority_ledger.decisions(id),
  action_code text NOT NULL, explanation text NOT NULL, operator_reference text NOT NULL, completed_unix_nano bigint NOT NULL,
  supporting_evidence bytea, commitment bytea NOT NULL CHECK(octet_length(commitment)=32), canonical_version smallint NOT NULL CHECK(canonical_version=1), created_at timestamptz NOT NULL DEFAULT now());

CREATE TABLE authority_ledger.response_streams (
  response_id text PRIMARY KEY, event_id text NOT NULL REFERENCES authority_ledger.authority_assertions(id),
  organization_id text NOT NULL REFERENCES authority_ledger.organizations(id), created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(event_id,organization_id), UNIQUE(response_id,event_id,organization_id));
CREATE TABLE authority_ledger.response_versions (
  version_id text PRIMARY KEY, response_id text NOT NULL, event_id text NOT NULL, organization_id text NOT NULL,
  previous_version_id text, state text NOT NULL CHECK(state IN ('RECEIVED','UNDER_REVIEW','DECIDED','ACTION_COMPLETED')),
  receipt_unix_nano bigint NOT NULL, policy_version_id text REFERENCES authority_ledger.policies(version_id),
  decision_id text REFERENCES authority_ledger.decisions(id), action_id text REFERENCES authority_ledger.actions(id),
  commitment bytea NOT NULL CHECK(octet_length(commitment)=32), canonical_version smallint NOT NULL CHECK(canonical_version=1), created_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(version_id,response_id,event_id,organization_id),
  FOREIGN KEY(response_id,event_id,organization_id) REFERENCES authority_ledger.response_streams(response_id,event_id,organization_id),
  FOREIGN KEY(previous_version_id,response_id,event_id,organization_id) REFERENCES authority_ledger.response_versions(version_id,response_id,event_id,organization_id));
CREATE UNIQUE INDEX response_versions_one_root ON authority_ledger.response_versions(response_id) WHERE previous_version_id IS NULL;
CREATE UNIQUE INDEX response_versions_one_successor ON authority_ledger.response_versions(response_id,previous_version_id) WHERE previous_version_id IS NOT NULL;

CREATE TABLE authority_ledger.ledger_submissions (
  operation_id text PRIMARY KEY CHECK(operation_id <> ''), operation_type text NOT NULL CHECK(operation_type IN ('AUTHORITY_ASSERTION','RESPONSE_VERSION')),
  domain_version_id text NOT NULL, contract_action text NOT NULL, signer_context text NOT NULL,
  intended_commitment bytea NOT NULL CHECK(octet_length(intended_commitment)=32),
  state text NOT NULL CHECK(state IN ('LOCAL_PENDING','SUBMITTED','CONFIRMED','SUBMISSION_FAILED')),
  transaction_hash bytea CHECK(transaction_hash IS NULL OR octet_length(transaction_hash)=32),
  block_number bigint CHECK(block_number IS NULL OR block_number>=0), block_hash bytea CHECK(block_hash IS NULL OR octet_length(block_hash)=32),
  attempt_count integer NOT NULL DEFAULT 0 CHECK(attempt_count>=0), last_attempt_at timestamptz, last_error_class text,
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(operation_type,domain_version_id),
  CHECK ((state='LOCAL_PENDING' AND transaction_hash IS NULL AND block_number IS NULL AND block_hash IS NULL) OR
         (state='SUBMITTED' AND transaction_hash IS NOT NULL AND block_number IS NULL AND block_hash IS NULL) OR
         (state='CONFIRMED' AND transaction_hash IS NOT NULL AND block_number IS NOT NULL AND block_hash IS NOT NULL) OR
         (state='SUBMISSION_FAILED' AND block_number IS NULL AND block_hash IS NULL)));
CREATE INDEX ledger_submissions_unresolved ON authority_ledger.ledger_submissions(state,created_at) WHERE state IN ('LOCAL_PENDING','SUBMITTED','SUBMISSION_FAILED');
