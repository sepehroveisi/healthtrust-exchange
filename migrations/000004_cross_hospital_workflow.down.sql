DROP TABLE IF EXISTS outgoing_access_requests;
DROP TABLE IF EXISTS incoming_referrals;
ALTER TABLE consents DROP COLUMN IF EXISTS referral_id;
ALTER TABLE access_requests DROP COLUMN IF EXISTS referral_id;
ALTER TABLE referrals DROP COLUMN IF EXISTS record_id;
