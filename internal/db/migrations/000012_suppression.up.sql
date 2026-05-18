-- Suppression: the tenant-plane tables behind automatic and manual
-- suppression. Every table carries tenant_id and is protected by Row-Level
-- Security, reusing the tenant_isolation pattern from 000004.

-- suppression_list holds the addresses that must not be mailed for a tenant.
-- An address is suppressed at most once per tenant (UNIQUE), so a later event
-- of a different reason does not duplicate the row.
CREATE TABLE suppression_list (
    id              uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email           text        NOT NULL,
    reason          text        NOT NULL
                                CHECK (reason IN ('hard_bounce', 'complaint', 'manual')),
    source_event_id uuid        REFERENCES delivery_events (id) ON DELETE SET NULL,
    suppressed_at   timestamptz NOT NULL DEFAULT now(),
    note            text        NOT NULL DEFAULT '',
    UNIQUE (tenant_id, email)
);

ALTER TABLE suppression_list ENABLE ROW LEVEL SECURITY;
ALTER TABLE suppression_list FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON suppression_list
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- bounce_settings holds a tenant's bounce-action configuration. A row is
-- created lazily; until then the column defaults (both toggles on) apply.
CREATE TABLE bounce_settings (
    tenant_id               uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    suppress_on_hard_bounce boolean     NOT NULL DEFAULT true,
    suppress_on_complaint   boolean     NOT NULL DEFAULT true,
    updated_at              timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE bounce_settings ENABLE ROW LEVEL SECURITY;
ALTER TABLE bounce_settings FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON bounce_settings
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON suppression_list, bounce_settings TO nvelope_app;
