-- Control-plane schema: platform identity and tenancy metadata. These tables
-- describe accounts, tenants, memberships, and invitations themselves — not
-- tenant-scoped business data — so they carry no Row-Level Security.

CREATE TABLE platform_users (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    email         citext      NOT NULL UNIQUE,
    password_hash text        NOT NULL,
    name          text        NOT NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE platform_sessions (
    id               uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    platform_user_id uuid        NOT NULL REFERENCES platform_users (id) ON DELETE CASCADE,
    token_hash       text        NOT NULL UNIQUE,
    created_at       timestamptz NOT NULL DEFAULT now(),
    expires_at       timestamptz NOT NULL,
    revoked_at       timestamptz
);

CREATE INDEX platform_sessions_user_idx ON platform_sessions (platform_user_id);

CREATE TABLE tenants (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    slug       citext      NOT NULL UNIQUE,
    name       text        NOT NULL CHECK (length(btrim(name)) > 0),
    status     text        NOT NULL DEFAULT 'active'
                           CHECK (status IN ('active', 'suspended', 'deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE platform_user_tenants (
    platform_user_id uuid        NOT NULL REFERENCES platform_users (id) ON DELETE CASCADE,
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    role             text        NOT NULL DEFAULT 'admin'
                                 CHECK (role IN ('owner', 'admin')),
    created_at       timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (platform_user_id, tenant_id)
);

CREATE INDEX platform_user_tenants_tenant_idx ON platform_user_tenants (tenant_id);

CREATE TABLE invitations (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email       citext      NOT NULL,
    role        text        NOT NULL DEFAULT 'admin'
                            CHECK (role IN ('owner', 'admin')),
    token_hash  text        NOT NULL UNIQUE,
    status      text        NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending', 'accepted', 'revoked', 'expired')),
    invited_by  uuid        NOT NULL REFERENCES platform_users (id),
    accepted_by uuid        REFERENCES platform_users (id),
    created_at  timestamptz NOT NULL DEFAULT now(),
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz
);

CREATE INDEX invitations_tenant_idx ON invitations (tenant_id);

-- At most one open invitation per email address per tenant.
CREATE UNIQUE INDEX invitations_pending_email_idx
    ON invitations (tenant_id, email) WHERE status = 'pending';

GRANT SELECT, INSERT, UPDATE, DELETE
    ON platform_users, platform_sessions, tenants, platform_user_tenants, invitations
    TO nvelope_app;
