CREATE TABLE IF NOT EXISTS organizations (
    id text PRIMARY KEY,
    name text NOT NULL,
    status text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS actors (
    id text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id),
    role text NOT NULL,
    status text NOT NULL,
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS blocks (
    height bigint PRIMARY KEY CHECK (height >= 0),
    hash bytea NOT NULL UNIQUE CHECK (octet_length(hash) = 32),
    previous_hash bytea NOT NULL CHECK (octet_length(previous_hash) = 32),
    timestamp_unix_nano bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS transactions (
    id text PRIMARY KEY,
    block_height bigint NOT NULL REFERENCES blocks(height) ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    type text NOT NULL,
    actor_id text NOT NULL REFERENCES actors(id),
    organization_id text NOT NULL REFERENCES organizations(id),
    resource_id text NOT NULL,
    payload_hash bytea NOT NULL CHECK (octet_length(payload_hash) = 32),
    timestamp_unix_nano bigint NOT NULL,
    public_key bytea NOT NULL CHECK (octet_length(public_key) = 32),
    signature bytea NOT NULL CHECK (octet_length(signature) = 64),
    UNIQUE (block_height, position)
);

CREATE INDEX IF NOT EXISTS transactions_block_height_idx ON transactions(block_height, position);
