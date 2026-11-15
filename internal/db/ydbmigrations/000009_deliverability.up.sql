-- Deliverability context (internal/deliverability): the staged inbound
-- delivery-feedback notifications, the attributed delivery events, the
-- tenant's suppression list and bounce settings, and the pre-computed campaign
-- analytics. No column has a DEFAULT, no CHECK and no foreign key: the adapters
-- write every value and the domain keeps them valid (see 000008_campaign).

-- inbound_feedback_events stages every notification before its tenant is
-- known, so it is the one table here without a tenant_id. raw_payload is JSON
-- that is only stored and read back whole, so it is a plain Utf8 column.
-- idx_inbound_dedupe is the idempotency key the consumer stages by.
-- idx_inbound_status serves the recovery sweep, which lists the notifications
-- still pending after a threshold.
CREATE TABLE IF NOT EXISTS inbound_feedback_events (
    id                  Utf8      NOT NULL,
    dedupe_key          Utf8      NOT NULL,
    event_kind          Utf8      NOT NULL,
    provider_message_id Utf8      NOT NULL,
    recipient_email     Utf8      NOT NULL,
    occurred_at         Timestamp NOT NULL,
    raw_payload         Utf8      NOT NULL,
    status              Utf8      NOT NULL,
    received_at         Timestamp NOT NULL,
    processed_at        Timestamp,
    PRIMARY KEY (id),
    INDEX idx_inbound_dedupe GLOBAL UNIQUE SYNC ON (dedupe_key),
    INDEX idx_inbound_status GLOBAL SYNC ON (status, received_at)
);

-- delivery_events holds the attributed feedback events. Exactly one of
-- campaign_recipient_id and transactional_message_id is set, which the domain
-- guarantees. A campaign's recipients go when their subscriber is deleted, so
-- campaign_recipient_id can dangle; nothing reads it. idx_delivery_events_inbound
-- makes RecordEvent idempotent on the inbound event.
CREATE TABLE IF NOT EXISTS delivery_events (
    tenant_id                Utf8      NOT NULL,
    id                       Utf8      NOT NULL,
    inbound_event_id         Utf8      NOT NULL,
    event_kind               Utf8      NOT NULL,
    recipient_email          Utf8      NOT NULL,
    campaign_id              Utf8,
    campaign_recipient_id    Utf8,
    transactional_message_id Utf8,
    provider_message_id      Utf8      NOT NULL,
    occurred_at              Timestamp NOT NULL,
    created_at               Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_delivery_events_inbound GLOBAL UNIQUE SYNC ON (tenant_id, inbound_event_id)
);

-- suppression_list: one row per suppressed address and tenant, which
-- idx_suppression_email enforces and the pre-send check reads. The List port
-- pages in id order, so the primary key is (tenant_id, id).
CREATE TABLE IF NOT EXISTS suppression_list (
    tenant_id       Utf8      NOT NULL,
    id              Utf8      NOT NULL,
    email           Utf8      NOT NULL,
    reason          Utf8      NOT NULL,
    source_event_id Utf8,
    suppressed_at   Timestamp NOT NULL,
    note            Utf8      NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_suppression_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- bounce_settings: a row is created lazily; until then the domain defaults
-- apply.
CREATE TABLE IF NOT EXISTS bounce_settings (
    tenant_id               Utf8      NOT NULL,
    suppress_on_hard_bounce Bool      NOT NULL,
    suppress_on_complaint   Bool      NOT NULL,
    updated_at              Timestamp NOT NULL,
    PRIMARY KEY (tenant_id)
);

-- campaign_analytics: the per-campaign roll-up the refresh job recomputes.
CREATE TABLE IF NOT EXISTS campaign_analytics (
    tenant_id        Utf8      NOT NULL,
    campaign_id      Utf8      NOT NULL,
    sent_count       Int64     NOT NULL,
    delivered_count  Int64     NOT NULL,
    opened_count     Int64     NOT NULL,
    clicked_count    Int64     NOT NULL,
    bounced_count    Int64     NOT NULL,
    complained_count Int64     NOT NULL,
    refreshed_at     Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, campaign_id)
);
