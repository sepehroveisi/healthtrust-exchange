DROP INDEX IF EXISTS patients_network_identity_idx;
ALTER TABLE outgoing_access_requests DROP COLUMN IF EXISTS purpose;
ALTER TABLE outgoing_access_requests DROP COLUMN IF EXISTS discovery_scope_id;
ALTER TABLE outgoing_access_requests DROP COLUMN IF EXISTS network_patient_id;
ALTER TABLE access_requests DROP COLUMN IF EXISTS purpose;
ALTER TABLE access_requests DROP COLUMN IF EXISTS source_organization_id;
ALTER TABLE access_requests DROP COLUMN IF EXISTS discovery_scope_id;
ALTER TABLE access_requests DROP COLUMN IF EXISTS network_patient_id;
ALTER TABLE patients DROP COLUMN IF EXISTS network_patient_id;
