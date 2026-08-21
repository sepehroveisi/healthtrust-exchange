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
ALTER TABLE outgoing_access_requests ADD COLUMN IF NOT EXISTS purpose text NOT NULL DEFAULT '';
