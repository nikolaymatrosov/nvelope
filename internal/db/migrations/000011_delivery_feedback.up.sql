-- Delivery feedback: the control-plane staging table for inbound delivery
-- notifications, the tenant-plane attributed delivery_events and
-- transactional_messages tables, and the provider-message-id wiring that ties
-- a notification back to the originating send.

-- inbound_feedback_events is a control-plane (non-tenant) table: it stages
-- every notification the stream consumer reads, before the owning tenant is
-- known. It has no RLS — it is written and read by cmd/consumer and the
-- feedback.process worker through the pool, since the tenant is not yet
-- resolved at ingestion.
CREATE TABLE inbound_feedback_events (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    dedupe_key          text        NOT NULL UNIQUE,
    event_kind          text        NOT NULL
                                    CHECK (event_kind IN ('bounce', 'complaint',
                                                          'delivery', 'open', 'click')),
    provider_message_id text        NOT NULL,
    recipient_email     text        NOT NULL,
    occurred_at         timestamptz NOT NULL,
    raw_payload         jsonb       NOT NULL,
    status              text        NOT NULL DEFAULT 'pending'
                                    CHECK (status IN ('pending', 'attributed', 'unattributed', 'failed')),
    received_at         timestamptz NOT NULL DEFAULT now(),
    processed_at        timestamptz
);

CREATE INDEX inbound_feedback_events_status_idx      ON inbound_feedback_events (status);
CREATE INDEX inbound_feedback_events_received_at_idx ON inbound_feedback_events (received_at);

GRANT SELECT, INSERT, UPDATE ON inbound_feedback_events TO nvelope_app;

-- transactional_messages records each transactional send so a transactional
-- bounce/complaint/delivery/open/click can be attributed. Tenant-plane,
-- RLS-protected.
CREATE TABLE transactional_messages (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    template_id         uuid        REFERENCES templates (id) ON DELETE SET NULL,
    provider_message_id text        NOT NULL UNIQUE,
    recipient_email     text        NOT NULL,
    sent_at             timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE transactional_messages ENABLE ROW LEVEL SECURITY;
ALTER TABLE transactional_messages FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON transactional_messages
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- delivery_events holds the attributed feedback events — bounces, complaints,
-- deliveries, opens, and clicks. Tenant-plane, RLS-protected. Exactly one of
-- campaign_recipient_id / transactional_message_id is set.
CREATE TABLE delivery_events (
    id                       uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id                uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    inbound_event_id         uuid        NOT NULL UNIQUE REFERENCES inbound_feedback_events (id),
    event_kind               text        NOT NULL
                                         CHECK (event_kind IN ('bounce', 'complaint',
                                                               'delivery', 'open', 'click')),
    recipient_email          text        NOT NULL,
    campaign_id              uuid        REFERENCES campaigns (id) ON DELETE SET NULL,
    campaign_recipient_id    uuid        REFERENCES campaign_recipients (id) ON DELETE SET NULL,
    transactional_message_id uuid        REFERENCES transactional_messages (id) ON DELETE SET NULL,
    provider_message_id      text        NOT NULL,
    occurred_at              timestamptz NOT NULL,
    created_at               timestamptz NOT NULL DEFAULT now(),
    CHECK ((campaign_recipient_id IS NOT NULL) <> (transactional_message_id IS NOT NULL))
);

ALTER TABLE delivery_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE delivery_events FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON delivery_events
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX delivery_events_campaign_kind_idx ON delivery_events (campaign_id, event_kind);
CREATE INDEX delivery_events_recipient_idx     ON delivery_events (tenant_id, recipient_email);

GRANT SELECT, INSERT, UPDATE, DELETE ON transactional_messages, delivery_events TO nvelope_app;

-- campaign_recipients gains the provider message id returned at send time and
-- the 'skipped' status used by the pre-send suppression check.
ALTER TABLE campaign_recipients ADD COLUMN provider_message_id text;

ALTER TABLE campaign_recipients DROP CONSTRAINT campaign_recipients_status_check;
ALTER TABLE campaign_recipients ADD CONSTRAINT campaign_recipients_status_check
    CHECK (status IN ('pending', 'sent', 'failed', 'skipped'));

CREATE INDEX campaign_recipients_provider_message_id_idx
    ON campaign_recipients (provider_message_id);

-- feedback_tenant_for_message resolves the owning tenant of a provider message
-- id without an app.tenant_id bound, mirroring the Phase 3 tracking_tenant_for_*
-- functions. It returns one tenant_id and leaks no row data.
CREATE FUNCTION feedback_tenant_for_message(p_provider_message_id text) RETURNS uuid
    LANGUAGE sql SECURITY DEFINER STABLE
    SET search_path = public
    AS $$
        SELECT tenant_id FROM (
            SELECT tenant_id FROM campaign_recipients
                WHERE provider_message_id = p_provider_message_id
            UNION ALL
            SELECT tenant_id FROM transactional_messages
                WHERE provider_message_id = p_provider_message_id
        ) AS matches
        LIMIT 1
    $$;

REVOKE ALL ON FUNCTION feedback_tenant_for_message(text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION feedback_tenant_for_message(text) TO nvelope_app;
