-- Phase 7: visual email editor + subscriber custom-field registry.
--
-- Three additions:
--   1. templates and campaigns gain `body_doc` (structured block document the
--      visual editor authors) and `theme` (per-row explicit theme override).
--      Both are NULL on legacy rows; the existing `body_html`/`body_text`
--      columns remain the canonical send-pipeline input.
--   2. `subscriber_fields` — tenant-scoped registry of custom subscriber fields.
--      Each definition has a stable `slug` used in `{{ subscriber.<slug> }}`
--      placeholders, a `display_name`, a `type`, an optional `default_value`,
--      and a `position` for the operator-managed ordering.
--      Built-in fields (email, name, first_name, last_name, state) are NOT
--      stored here; they are surfaced as pseudo-rows by the query layer so a
--      tenant cannot delete or rename them.

ALTER TABLE templates
    ADD COLUMN body_doc jsonb,
    ADD COLUMN theme    jsonb;

ALTER TABLE campaigns
    ADD COLUMN body_doc jsonb,
    ADD COLUMN theme    jsonb;

CREATE TABLE subscriber_fields (
    id            uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id     uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    slug          text        NOT NULL,
    display_name  text        NOT NULL,
    type          text        NOT NULL CHECK (type IN ('text', 'number', 'date', 'boolean', 'url')),
    default_value text        NOT NULL DEFAULT '',
    position      integer     NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),

    UNIQUE (tenant_id, slug),
    CHECK (slug ~ '^[a-z][a-z0-9_]{0,62}$'),
    CHECK (length(display_name) BETWEEN 1 AND 128)
);

-- Listing fields newest-first per tenant, and for the operator-managed
-- ordering used by the merge-tag picker and the Phase 6 subscription-page
-- field picker.
CREATE INDEX subscriber_fields_tenant_position_idx
    ON subscriber_fields (tenant_id, position, created_at);

ALTER TABLE subscriber_fields ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriber_fields FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscriber_fields
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON subscriber_fields TO nvelope_app;
