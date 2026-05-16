-- tenant_settings is the first tenant-plane table. It carries tenant_id and is
-- protected by Row-Level Security — the pattern every later tenant-plane table
-- reuses. One row per tenant, created with the tenant.

CREATE TABLE tenant_settings (
    tenant_id    uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    display_name text        NOT NULL,
    timezone     text        NOT NULL DEFAULT 'UTC',
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

-- ENABLE turns RLS on; FORCE applies it even to the table owner, so no role
-- (short of superuser/BYPASSRLS) can read or write across tenants.
ALTER TABLE tenant_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_settings FORCE  ROW LEVEL SECURITY;

-- With app.tenant_id unset, current_setting(..., true) is NULL and the
-- predicate matches no rows — isolation fails closed (deny by default).
CREATE POLICY tenant_isolation ON tenant_settings
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_settings TO nvelope_app;
