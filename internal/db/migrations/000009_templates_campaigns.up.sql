-- Templates and campaigns: the tenant-plane tables behind authoring and
-- sending a campaign. Every table carries tenant_id and is protected by
-- Row-Level Security, reusing the tenant_isolation pattern from 000004.

CREATE TABLE templates (
    id         uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id  uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name       text        NOT NULL,
    kind       text        NOT NULL CHECK (kind IN ('campaign', 'transactional')),
    subject    text        NOT NULL,
    body_html  text        NOT NULL,
    body_text  text        NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

ALTER TABLE templates ENABLE ROW LEVEL SECURITY;
ALTER TABLE templates FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON templates
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE campaigns (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name              text        NOT NULL,
    subject           text        NOT NULL,
    body_html         text        NOT NULL,
    body_text         text        NOT NULL DEFAULT '',
    from_name         text        NOT NULL,
    from_local_part   text        NOT NULL,
    sending_domain_id uuid        REFERENCES sending_domains (id) ON DELETE SET NULL,
    template_id       uuid        REFERENCES templates (id) ON DELETE SET NULL,
    status            text        NOT NULL DEFAULT 'draft'
                                  CHECK (status IN ('draft', 'running', 'paused', 'finished', 'cancelled')),
    max_send_errors   integer     NOT NULL DEFAULT 100,
    sent_count        integer     NOT NULL DEFAULT 0,
    failed_count      integer     NOT NULL DEFAULT 0,
    recipient_count   integer     NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    started_at        timestamptz,
    finished_at       timestamptz
);

ALTER TABLE campaigns ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaigns FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaigns
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX campaigns_tenant_status_idx ON campaigns (tenant_id, status);

CREATE TABLE campaign_lists (
    campaign_id   uuid  NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    tenant_id     uuid  NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    list_id       uuid  REFERENCES lists (id) ON DELETE CASCADE,
    segment_query jsonb,
    CHECK ((list_id IS NOT NULL) <> (segment_query IS NOT NULL)),
    UNIQUE (campaign_id, list_id)
);

ALTER TABLE campaign_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaign_lists FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaign_lists
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX campaign_lists_campaign_idx ON campaign_lists (campaign_id);

CREATE TABLE campaign_recipients (
    id             uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id      uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    campaign_id    uuid        NOT NULL REFERENCES campaigns (id) ON DELETE CASCADE,
    subscriber_id  uuid        NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    email          text        NOT NULL,
    status         text        NOT NULL DEFAULT 'pending'
                                CHECK (status IN ('pending', 'sent', 'failed')),
    failure_reason text        NOT NULL DEFAULT '',
    sent_at        timestamptz,
    UNIQUE (campaign_id, email)
);

ALTER TABLE campaign_recipients ENABLE ROW LEVEL SECURITY;
ALTER TABLE campaign_recipients FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON campaign_recipients
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX campaign_recipients_campaign_status_idx
    ON campaign_recipients (campaign_id, status);

GRANT SELECT, INSERT, UPDATE, DELETE
    ON templates, campaigns, campaign_lists, campaign_recipients TO nvelope_app;
