-- Phase 6 US3: campaign archive visibility + per-tenant branding for public
-- pages. The new tenant_branding table is RLS-protected on the same pattern
-- as every other tenant-plane table.

ALTER TABLE campaigns ADD COLUMN archive_visible boolean     NOT NULL DEFAULT false;
ALTER TABLE campaigns ADD COLUMN archived_at     timestamptz;

-- A partial index supports the archive index and RSS feed reading every
-- archive-visible campaign of a tenant newest-first.
CREATE INDEX campaigns_archive_idx
    ON campaigns (tenant_id, archived_at DESC)
    WHERE archive_visible = true;

CREATE TABLE tenant_branding (
    tenant_id     uuid        PRIMARY KEY REFERENCES tenants (id) ON DELETE CASCADE,
    logo_url      text        NOT NULL DEFAULT '',
    primary_color text        NOT NULL DEFAULT '',
    custom_css    text        NOT NULL DEFAULT '',
    updated_at    timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE tenant_branding ENABLE ROW LEVEL SECURITY;
ALTER TABLE tenant_branding FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON tenant_branding
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON tenant_branding TO nvelope_app;
