ALTER TABLE referrals ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
ALTER TABLE incoming_referrals ADD COLUMN IF NOT EXISTS network_patient_id text NOT NULL DEFAULT '';
