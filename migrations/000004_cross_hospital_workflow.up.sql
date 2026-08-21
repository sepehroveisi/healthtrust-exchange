ALTER TABLE referrals ADD COLUMN IF NOT EXISTS record_id text;
ALTER TABLE access_requests ADD COLUMN IF NOT EXISTS referral_id text;
ALTER TABLE consents ADD COLUMN IF NOT EXISTS referral_id text;
CREATE TABLE IF NOT EXISTS incoming_referrals(id text PRIMARY KEY,patient_id text NOT NULL,record_id text NOT NULL,source_organization_id text NOT NULL,destination_organization_id text NOT NULL,created_by_actor_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,commit_state text NOT NULL,transaction_id text NOT NULL,received_at_unix_nano bigint NOT NULL);
CREATE TABLE IF NOT EXISTS outgoing_access_requests(id text PRIMARY KEY,referral_id text NOT NULL,patient_id text NOT NULL,record_id text NOT NULL,requester_actor_id text NOT NULL,requester_organization_id text NOT NULL,source_organization_id text NOT NULL,destination_organization_id text NOT NULL,status text NOT NULL,created_at_unix_nano bigint NOT NULL,updated_at_unix_nano bigint NOT NULL);
CREATE INDEX IF NOT EXISTS incoming_referrals_patient_idx ON incoming_referrals(patient_id,received_at_unix_nano);
CREATE INDEX IF NOT EXISTS outgoing_access_patient_idx ON outgoing_access_requests(patient_id,created_at_unix_nano);
