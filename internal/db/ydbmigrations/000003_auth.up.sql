-- Auth (internal/auth): platform users, login sessions and email-verification
-- challenges. Control-plane tables, not tenant-scoped, so there is no
-- tenant_id column.

-- email keeps the address as entered. email_key is its lower-cased form and
-- carries the uniqueness, standing in for the Postgres citext column.
CREATE TABLE IF NOT EXISTS platform_users (
    id                Utf8      NOT NULL,
    email             Utf8      NOT NULL,
    email_key         Utf8      NOT NULL,
    password_hash     Utf8      NOT NULL,
    name              Utf8      NOT NULL,
    locale            Utf8,
    email_verified_at Timestamp,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_platform_users_email_key GLOBAL UNIQUE SYNC ON (email_key)
);

-- A session is only ever looked up by its token hash, so the hash is the
-- primary key. TTL deletes a session once it has expired.
CREATE TABLE IF NOT EXISTS platform_sessions (
    token_hash       Utf8      NOT NULL,
    id               Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    created_at       Timestamp NOT NULL,
    expires_at       Timestamp NOT NULL,
    revoked_at       Timestamp,
    PRIMARY KEY (token_hash)
) WITH (TTL = Interval("PT0S") ON expires_at);

-- TTL replaces the verification-cleanup job: a challenge is deleted once its
-- validity window has closed, which is what the job's sweep does on Postgres.
CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id          Utf8      NOT NULL,
    user_id     Utf8      NOT NULL,
    token_hash  Utf8      NOT NULL,
    expires_at  Timestamp NOT NULL,
    created_at  Timestamp NOT NULL,
    consumed_at Timestamp,
    PRIMARY KEY (id),
    INDEX idx_email_verification_tokens_token_hash GLOBAL UNIQUE SYNC ON (token_hash),
    INDEX idx_email_verification_tokens_user GLOBAL SYNC ON (user_id)
) WITH (TTL = Interval("PT0S") ON expires_at);
