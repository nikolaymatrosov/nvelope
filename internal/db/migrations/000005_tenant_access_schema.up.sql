-- The iam tenant-plane tables: tenant-scoped users, working sessions, roles,
-- role assignments, API keys, TOTP recovery codes, and the audit log. Every
-- table carries tenant_id and is protected by Row-Level Security, reusing the
-- tenant_isolation pattern from 000004.
--
-- This migration applies before 000006 (audience): user_list_roles references
-- lists, so its foreign key to lists(id) is added by 000006 once that table
-- exists. The api_keys and recovery_codes columns are exercised in US5.

CREATE TABLE users (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    platform_user_id uuid        NOT NULL REFERENCES platform_users (id) ON DELETE CASCADE,
    email            citext      NOT NULL,
    name             text        NOT NULL DEFAULT '',
    status           text        NOT NULL DEFAULT 'active'
                                 CHECK (status IN ('active', 'suspended')),
    totp_enabled     boolean     NOT NULL DEFAULT false,
    totp_secret      bytea,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, platform_user_id),
    UNIQUE (tenant_id, email)
);

ALTER TABLE users ENABLE ROW LEVEL SECURITY;
ALTER TABLE users FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON users
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE sessions (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id    uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash text        NOT NULL UNIQUE,
    state      text        NOT NULL DEFAULT 'active'
                           CHECK (state IN ('totp-pending', 'active', 'revoked')),
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz
);

ALTER TABLE sessions ENABLE ROW LEVEL SECURITY;
ALTER TABLE sessions FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sessions
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE roles (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    permissions text[]      NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

ALTER TABLE roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE roles FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON roles
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE user_roles (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id   uuid NOT NULL REFERENCES roles (id),
    PRIMARY KEY (tenant_id, user_id)
);

ALTER TABLE user_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_roles FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_roles
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- list_id has no foreign key here: lists is created by 000006, which adds the
-- constraint once the table exists.
CREATE TABLE user_list_roles (
    tenant_id uuid NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id   uuid NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    list_id   uuid NOT NULL,
    role_id   uuid NOT NULL REFERENCES roles (id),
    PRIMARY KEY (tenant_id, user_id, list_id)
);

ALTER TABLE user_list_roles ENABLE ROW LEVEL SECURITY;
ALTER TABLE user_list_roles FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON user_list_roles
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE api_keys (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name         text        NOT NULL,
    token_hash   text        NOT NULL UNIQUE,
    permissions  text[]      NOT NULL DEFAULT '{}',
    created_by   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    created_at   timestamptz NOT NULL DEFAULT now(),
    last_used_at timestamptz,
    revoked_at   timestamptz
);

ALTER TABLE api_keys ENABLE ROW LEVEL SECURITY;
ALTER TABLE api_keys FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON api_keys
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE recovery_codes (
    tenant_id uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    user_id   uuid        NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    code_hash text        NOT NULL,
    used_at   timestamptz,
    PRIMARY KEY (tenant_id, user_id, code_hash)
);

ALTER TABLE recovery_codes ENABLE ROW LEVEL SECURITY;
ALTER TABLE recovery_codes FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON recovery_codes
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE audit_log (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    actor_id   uuid        NOT NULL,
    actor_kind text        NOT NULL CHECK (actor_kind IN ('session', 'api-key')),
    action     text        NOT NULL,
    target     text        NOT NULL DEFAULT '',
    metadata   jsonb       NOT NULL DEFAULT '{}',
    created_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE audit_log ENABLE ROW LEVEL SECURITY;
ALTER TABLE audit_log FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON audit_log
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX audit_log_tenant_created_idx ON audit_log (tenant_id, created_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE
    ON users, sessions, roles, user_roles, user_list_roles, api_keys, recovery_codes, audit_log
    TO nvelope_app;
