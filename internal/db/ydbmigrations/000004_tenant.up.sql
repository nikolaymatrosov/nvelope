-- Tenant (internal/tenant): workspaces, memberships, invitations, settings and
-- branding. tenants is a control-plane table keyed by id. The other four are
-- tenant-scoped: their primary key starts with tenant_id and every query
-- filters by it. No column has a DEFAULT, so the adapters write the values
-- Postgres takes from its column defaults.

-- slug is stored lower-cased (the domain lower-cases it), so the unique index
-- reproduces the uniqueness of the Postgres citext column.
CREATE TABLE IF NOT EXISTS tenants (
    id         Utf8      NOT NULL,
    slug       Utf8      NOT NULL,
    name       Utf8      NOT NULL,
    status     Utf8      NOT NULL,
    created_at Timestamp NOT NULL,
    updated_at Timestamp NOT NULL,
    PRIMARY KEY (id),
    INDEX idx_tenants_slug GLOBAL UNIQUE SYNC ON (slug)
);

-- Keyed by tenant first for ListMembers and GetMembershipRole. The user index
-- serves ListMembershipsForUser.
CREATE TABLE IF NOT EXISTS platform_user_tenants (
    tenant_id        Utf8      NOT NULL,
    platform_user_id Utf8      NOT NULL,
    role             Utf8      NOT NULL,
    created_at       Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, platform_user_id),
    INDEX idx_platform_user_tenants_user GLOBAL SYNC ON (platform_user_id)
);

-- email keeps the address as entered and email_key is its lower-cased form,
-- standing in for the Postgres citext column. Postgres allows one pending
-- invitation per (tenant_id, email) through a partial unique index. YDB has no
-- partial index, so YDBInvitations.Create checks for a pending row and inserts
-- in one serializable transaction. The accept link carries only the token, so
-- token_hash has a global unique index.
CREATE TABLE IF NOT EXISTS invitations (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    email       Utf8      NOT NULL,
    email_key   Utf8      NOT NULL,
    role        Utf8      NOT NULL,
    token_hash  Utf8      NOT NULL,
    status      Utf8      NOT NULL,
    invited_by  Utf8      NOT NULL,
    accepted_by Utf8,
    created_at  Timestamp NOT NULL,
    expires_at  Timestamp NOT NULL,
    accepted_at Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_invitations_token_hash GLOBAL UNIQUE SYNC ON (token_hash)
);

-- One row per tenant, created with the tenant.
CREATE TABLE IF NOT EXISTS tenant_settings (
    tenant_id    Utf8      NOT NULL,
    display_name Utf8      NOT NULL,
    timezone     Utf8      NOT NULL,
    -- 'standard' strips active content from raw HTML on save; 'off' stores
    -- it verbatim. The domain enforces the values (YDB has no CHECK).
    raw_html_sanitization Utf8 NOT NULL,
    created_at   Timestamp NOT NULL,
    updated_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);

-- At most one row per tenant. A tenant without a row uses the platform
-- defaults.
CREATE TABLE IF NOT EXISTS tenant_branding (
    tenant_id     Utf8      NOT NULL,
    logo_url      Utf8      NOT NULL,
    primary_color Utf8      NOT NULL,
    custom_css    Utf8      NOT NULL,
    updated_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);
