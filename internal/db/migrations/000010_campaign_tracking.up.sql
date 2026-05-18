-- Campaign open- and click-tracking: the tenant-plane tables that record a
-- campaign's distinct tracked links and the per-recipient open and click
-- events. Every table carries tenant_id and is protected by Row-Level
-- Security, reusing the tenant_isolation pattern from 000004.

CREATE TABLE links (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    campaign_id uuid        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    url         text        NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (campaign_id, url)
);

ALTER TABLE links ENABLE ROW LEVEL SECURITY;
ALTER TABLE links FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON links
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE link_clicks (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    link_id      uuid        NOT NULL REFERENCES links (id) ON DELETE CASCADE,
    campaign_id  uuid        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    recipient_id uuid        NOT NULL REFERENCES campaign_recipients (id) ON DELETE CASCADE,
    clicked_at   timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE link_clicks ENABLE ROW LEVEL SECURITY;
ALTER TABLE link_clicks FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON link_clicks
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX link_clicks_campaign_idx  ON link_clicks (campaign_id);
CREATE INDEX link_clicks_recipient_idx ON link_clicks (recipient_id);

CREATE TABLE campaign_views (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    campaign_id  uuid        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    recipient_id uuid        NOT NULL REFERENCES campaign_recipients (id) ON DELETE CASCADE,
    viewed_at    timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE campaign_views ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaign_views FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaign_views
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX campaign_views_campaign_idx  ON campaign_views (campaign_id);
CREATE INDEX campaign_views_recipient_idx ON campaign_views (recipient_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON links, link_clicks, campaign_views TO nvelope_app;

-- The public tracking endpoints resolve which tenant owns a link or campaign
-- UUID before opening the tenant-bound transaction. That lookup must read one
-- RLS-protected row without an app.tenant_id yet bound, so it goes through a
-- SECURITY DEFINER function owned by the privileged migration role, which
-- bypasses RLS. The function only ever returns a single tenant_id for a
-- caller-supplied UUID — it leaks no row data.
CREATE FUNCTION tracking_tenant_for_link(p_link_id uuid) RETURNS uuid
    LANGUAGE sql SECURITY DEFINER STABLE
    SET search_path = public
    AS $$ SELECT tenant_id FROM links WHERE id = p_link_id $$;

CREATE FUNCTION tracking_tenant_for_campaign(p_campaign_id uuid) RETURNS uuid
    LANGUAGE sql SECURITY DEFINER STABLE
    SET search_path = public
    AS $$ SELECT tenant_id FROM campaigns WHERE id = p_campaign_id $$;

REVOKE ALL ON FUNCTION tracking_tenant_for_link(uuid)     FROM PUBLIC;
REVOKE ALL ON FUNCTION tracking_tenant_for_campaign(uuid) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION tracking_tenant_for_link(uuid)     TO nvelope_app;
GRANT EXECUTE ON FUNCTION tracking_tenant_for_campaign(uuid) TO nvelope_app;
