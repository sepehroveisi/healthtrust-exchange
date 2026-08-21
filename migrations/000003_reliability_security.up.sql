CREATE TABLE IF NOT EXISTS idempotency_keys(operation text NOT NULL,key text NOT NULL,actor_id text NOT NULL,request_hash bytea NOT NULL CHECK(octet_length(request_hash)=32),status text NOT NULL,response_reference text,created_at timestamptz NOT NULL DEFAULT now(),completed_at timestamptz,PRIMARY KEY(operation,key));
CREATE TABLE IF NOT EXISTS consumed_requester_proofs(proof_id text PRIMARY KEY,actor_id text NOT NULL,organization_id text NOT NULL,record_id text NOT NULL,consumed_at timestamptz NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS consumed_peer_requests(request_id text PRIMARY KEY,source_node_id text NOT NULL,source_organization_id text NOT NULL,consumed_at timestamptz NOT NULL,expires_at timestamptz NOT NULL);
CREATE TABLE IF NOT EXISTS workflow_reconciliation(operation text NOT NULL,object_id text NOT NULL,retry_count integer NOT NULL DEFAULT 0,last_error text,updated_at timestamptz NOT NULL DEFAULT now(),PRIMARY KEY(operation,object_id));
CREATE INDEX IF NOT EXISTS requester_proof_expiry_idx ON consumed_requester_proofs(expires_at);
CREATE INDEX IF NOT EXISTS peer_request_expiry_idx ON consumed_peer_requests(expires_at);
