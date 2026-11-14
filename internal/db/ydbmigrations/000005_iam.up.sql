-- IAM (internal/iam): tenant-plane users, working sessions, roles, role
-- assignments, API keys, TOTP recovery codes and the audit log. Every table is
-- tenant-scoped: its primary key starts with tenant_id and every query filters
-- by it. No column has a DEFAULT, so the adapters write the values Postgres
-- takes from its column defaults. YDB has no foreign keys, and no iam query
-- reads a table another context owns.

-- email is stored lower-cased (domain.NewTenantUser lower-cases it), so the
-- unique index reproduces the uniqueness of the Postgres citext column.
CREATE TABLE IF NOT EXISTS users (
    tenant_id        Utf8      NOT NULL,
    id               Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    email            Utf8      NOT NULL,
    name             Utf8      NOT NULL,
    status           Utf8      NOT NULL,
    totp_enabled     Bool      NOT NULL,
    totp_secret      String,
    created_at       Timestamp NOT NULL,
    updated_at       Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_users_platform_user GLOBAL UNIQUE SYNC ON (tenant_id, platform_user_id),
    INDEX idx_users_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- A session is looked up by its token hash, which is globally unique. TTL
-- deletes a session once it has expired. An expired session authenticates
-- nothing (Session.IsActive), so no caller needs to read one back.
CREATE TABLE IF NOT EXISTS sessions (
    tenant_id  Utf8      NOT NULL,
    id         Utf8      NOT NULL,
    user_id    Utf8      NOT NULL,
    token_hash Utf8      NOT NULL,
    state      Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    expires_at Timestamp NOT NULL,
    revoked_at Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_sessions_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
) WITH (TTL = Interval("PT0S") ON expires_at);

-- permissions is a JSON array of permission strings, only stored and read back
-- whole, so it is a plain Utf8 column (Postgres: text[]). Role names are
-- case-sensitive, as on Postgres.
CREATE TABLE IF NOT EXISTS roles (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    name        Utf8      NOT NULL,
    permissions Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    updated_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_roles_tenant_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- At most one tenant-level role per user.
CREATE TABLE IF NOT EXISTS user_roles (
    tenant_id Utf8 NOT NULL,
    user_id   Utf8 NOT NULL,
    role_id   Utf8 NOT NULL,
    PRIMARY KEY (tenant_id, user_id)
);

-- At most one role per user and list.
CREATE TABLE IF NOT EXISTS user_list_roles (
    tenant_id Utf8 NOT NULL,
    user_id   Utf8 NOT NULL,
    list_id   Utf8 NOT NULL,
    role_id   Utf8 NOT NULL,
    PRIMARY KEY (tenant_id, user_id, list_id)
);

-- An API key is looked up by its token hash, which is globally unique.
CREATE TABLE IF NOT EXISTS api_keys (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    name         Utf8      NOT NULL,
    token_hash   Utf8      NOT NULL,
    permissions  Utf8      NOT NULL,
    created_by   Utf8      NOT NULL,
    created_at   Timestamp NOT NULL,
    last_used_at Timestamp,
    revoked_at   Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_api_keys_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
);

CREATE TABLE IF NOT EXISTS recovery_codes (
    tenant_id Utf8      NOT NULL,
    user_id   Utf8      NOT NULL,
    code_hash Utf8      NOT NULL,
    used_at   Timestamp,
    PRIMARY KEY (tenant_id, user_id, code_hash)
);

-- Keyed in the order AuditRepository.All pages through (Postgres: the
-- (tenant_id, created_at DESC) index). metadata is JSON that is only stored
-- and read back whole, so it is a plain Utf8 column. The deliverability
-- context writes to this table too.
CREATE TABLE IF NOT EXISTS audit_log (
    tenant_id  Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    id         Utf8      NOT NULL,
    actor_id   Utf8      NOT NULL,
    actor_kind Utf8      NOT NULL,
    action     Utf8      NOT NULL,
    target     Utf8      NOT NULL,
    metadata   Utf8      NOT NULL,
    PRIMARY KEY (tenant_id, created_at, id)
);
