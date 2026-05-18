-- Campaign analytics: a pre-computed, per-campaign roll-up of the six delivery
-- counts. This is a regular RLS-protected table, NOT a native materialized
-- view — a matview cannot carry a Row-Level Security policy (see research R4).
-- It is refreshed by the analytics.refresh River job.

CREATE TABLE campaign_analytics (
    campaign_id      uuid        PRIMARY KEY REFERENCES campaigns (id) ON DELETE CASCADE,
    tenant_id        uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    sent_count       integer     NOT NULL DEFAULT 0,
    delivered_count  integer     NOT NULL DEFAULT 0,
    opened_count     integer     NOT NULL DEFAULT 0,
    clicked_count    integer     NOT NULL DEFAULT 0,
    bounced_count    integer     NOT NULL DEFAULT 0,
    complained_count integer     NOT NULL DEFAULT 0,
    refreshed_at     timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE campaign_analytics ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaign_analytics FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaign_analytics
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX campaign_analytics_tenant_idx ON campaign_analytics (tenant_id);

GRANT SELECT, INSERT, UPDATE, DELETE ON campaign_analytics TO nvelope_app;
