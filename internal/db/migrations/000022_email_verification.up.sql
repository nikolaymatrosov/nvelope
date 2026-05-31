-- Email verification on registration.
--
-- A new platform account is created unverified and must prove control of its
-- email address by opening a single-use link before it can sign in.
--
-- `email_verified_at` is a control-plane account attribute (platform_users is
-- control-plane, so no Row-Level Security). It is nullable: NULL means the
-- account has not verified its email.
ALTER TABLE platform_users
    ADD COLUMN email_verified_at timestamptz;

-- Accounts that existed before this feature are out of scope: treat them as
-- already verified so the new sign-in gate does not lock them out.
UPDATE platform_users
    SET email_verified_at = now()
    WHERE email_verified_at IS NULL;

-- Single-use, time-bounded email-ownership challenges. Control-plane (no RLS),
-- like platform_users and sessions. The raw token is never stored — only its
-- SHA-256 hash. A consumed row is kept (not deleted) so the verify path can
-- tell "already verified" apart from "invalid or expired".
CREATE TABLE email_verification_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES platform_users (id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    consumed_at timestamptz
);

CREATE INDEX email_verification_tokens_user_id_idx
    ON email_verification_tokens (user_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON email_verification_tokens TO nvelope_app;
