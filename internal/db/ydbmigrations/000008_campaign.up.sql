-- Campaign context (internal/campaign): templates, campaigns and their send
-- targets, per-recipient send progress, tracked links with their click and
-- view events, and the record of transactional sends. Every table is
-- tenant-scoped: its primary key starts with tenant_id and every query filters
-- by it. No column has a DEFAULT, so the adapters write the values Postgres
-- takes from its column defaults. YDB has no foreign keys and no CHECK
-- constraints: the domain keeps statuses and kinds valid, and the adapters
-- write the cascades the port needs (see the spec, "Plan B7 planning spike").

-- name is unique per tenant, as UNIQUE (tenant_id, name) is on Postgres.
-- body_doc and theme are JSON only stored and read back whole.
CREATE TABLE IF NOT EXISTS templates (
    tenant_id  Utf8      NOT NULL,
    id         Utf8      NOT NULL,
    name       Utf8      NOT NULL,
    kind       Utf8      NOT NULL,
    subject    Utf8      NOT NULL,
    body_html  Utf8      NOT NULL,
    body_text  Utf8      NOT NULL,
    body_doc   Utf8,
    theme      Utf8,
    created_at Timestamp NOT NULL,
    updated_at Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_templates_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- idx_campaigns_id lets the public open-tracking endpoint find the owning
-- tenant of a campaign id, which Postgres does through a SECURITY DEFINER
-- function. sending_domain_id and template_id are NULL when unset.
CREATE TABLE IF NOT EXISTS campaigns (
    tenant_id         Utf8      NOT NULL,
    id                Utf8      NOT NULL,
    name              Utf8      NOT NULL,
    subject           Utf8      NOT NULL,
    body_html         Utf8      NOT NULL,
    body_text         Utf8      NOT NULL,
    from_name         Utf8      NOT NULL,
    from_local_part   Utf8      NOT NULL,
    sending_domain_id Utf8,
    template_id       Utf8,
    status            Utf8      NOT NULL,
    max_send_errors   Int64     NOT NULL,
    sent_count        Int64     NOT NULL,
    failed_count      Int64     NOT NULL,
    recipient_count   Int64     NOT NULL,
    body_doc          Utf8,
    theme             Utf8,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    started_at        Timestamp,
    finished_at       Timestamp,
    archive_visible   Bool      NOT NULL,
    archived_at       Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_campaigns_id GLOBAL UNIQUE SYNC ON (id)
);

-- A target is a list (list_id) or a segment (segment_query), never both.
-- idx_campaign_lists_unique reproduces UNIQUE (campaign_id, list_id): YDB
-- admits many NULL list_ids, as Postgres does. idx_campaign_lists_list serves
-- the delete of a list, which removes its targets.
CREATE TABLE IF NOT EXISTS campaign_lists (
    tenant_id     Utf8 NOT NULL,
    campaign_id   Utf8 NOT NULL,
    id            Utf8 NOT NULL,
    list_id       Utf8,
    segment_query Utf8,
    PRIMARY KEY (tenant_id, campaign_id, id),
    INDEX idx_campaign_lists_list GLOBAL SYNC ON (tenant_id, list_id),
    INDEX idx_campaign_lists_unique GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, list_id)
);

-- The primary key makes a campaign's recipients one key range, in id order,
-- which the batch window reads. idx_campaign_recipients_id finds a recipient
-- from its id alone (the send worker and the tracking endpoints know only the
-- id). The email index reproduces UNIQUE (campaign_id, email). The subscriber
-- index serves the delete of a subscriber, which removes its recipient rows.
-- The provider index lets delivery feedback find the recipient of a provider
-- message id, with or without the tenant (Plan B8).
CREATE TABLE IF NOT EXISTS campaign_recipients (
    tenant_id           Utf8      NOT NULL,
    campaign_id         Utf8      NOT NULL,
    id                  Utf8      NOT NULL,
    subscriber_id       Utf8      NOT NULL,
    email               Utf8      NOT NULL,
    status              Utf8      NOT NULL,
    failure_reason      Utf8      NOT NULL,
    sent_at             Timestamp,
    provider_message_id Utf8,
    PRIMARY KEY (tenant_id, campaign_id, id),
    INDEX idx_campaign_recipients_id GLOBAL SYNC ON (tenant_id, id),
    INDEX idx_campaign_recipients_email GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, email),
    INDEX idx_campaign_recipients_subscriber GLOBAL SYNC ON (tenant_id, subscriber_id),
    INDEX idx_campaign_recipients_provider GLOBAL SYNC ON (provider_message_id)
);

-- idx_links_id lets the public click-tracking endpoint find the owning tenant
-- of a link id. idx_links_url reproduces UNIQUE (campaign_id, url).
CREATE TABLE IF NOT EXISTS links (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    campaign_id Utf8      NOT NULL,
    url         Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_links_id GLOBAL UNIQUE SYNC ON (id),
    INDEX idx_links_url GLOBAL UNIQUE SYNC ON (tenant_id, campaign_id, url)
);

-- Click and view events are only written: no query reads them, so they carry
-- no index and need no cascade from a deleted recipient.
CREATE TABLE IF NOT EXISTS link_clicks (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    link_id      Utf8      NOT NULL,
    campaign_id  Utf8      NOT NULL,
    recipient_id Utf8      NOT NULL,
    clicked_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

CREATE TABLE IF NOT EXISTS campaign_views (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    campaign_id  Utf8      NOT NULL,
    recipient_id Utf8      NOT NULL,
    viewed_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);

-- provider_message_id is unique across tenants, as on Postgres: delivery
-- feedback resolves the owning tenant from it (Plan B8). template_id is only
-- written, so a deleted template leaves it dangling.
CREATE TABLE IF NOT EXISTS transactional_messages (
    tenant_id           Utf8      NOT NULL,
    id                  Utf8      NOT NULL,
    template_id         Utf8,
    provider_message_id Utf8      NOT NULL,
    recipient_email     Utf8      NOT NULL,
    sent_at             Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_transactional_messages_provider GLOBAL UNIQUE SYNC ON (provider_message_id)
);
