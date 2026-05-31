DROP TABLE IF EXISTS email_verification_tokens;

ALTER TABLE platform_users
    DROP COLUMN IF EXISTS email_verified_at;
