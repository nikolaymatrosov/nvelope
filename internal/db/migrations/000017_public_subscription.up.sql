-- Phase 6 US1: public subscription pages and the double-opt-in pending
-- subscriptions they create. Both tables carry tenant_id and are protected by
-- Row-Level Security, reusing the tenant_isolation pattern from 000004.

CREATE TABLE subscription_pages (
    id                uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id         uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    slug              text        NOT NULL,
    title             text        NOT NULL,
    target_list_ids   uuid[]      NOT NULL,
    fields            jsonb       NOT NULL DEFAULT '[]',
    sending_domain_id uuid        NOT NULL REFERENCES sending_domains (id) ON DELETE RESTRICT,
    from_name         text        NOT NULL,
    from_local_part   text        NOT NULL,
    active            boolean     NOT NULL DEFAULT true,
    created_at        timestamptz NOT NULL DEFAULT now(),
    updated_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, slug)
);

ALTER TABLE subscription_pages ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscription_pages FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscription_pages
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE pending_subscriptions (
    id                      uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id               uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subscription_page_id    uuid        NOT NULL REFERENCES subscription_pages (id) ON DELETE CASCADE,
    email                   citext      NOT NULL,
    attributes              jsonb       NOT NULL DEFAULT '{}',
    target_list_ids         uuid[]      NOT NULL,
    confirmation_token_hash text        NOT NULL UNIQUE,
    expires_at              timestamptz NOT NULL,
    created_at              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email, subscription_page_id)
);

ALTER TABLE pending_subscriptions ENABLE ROW LEVEL SECURITY;
ALTER TABLE pending_subscriptions FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON pending_subscriptions
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- The per-subscriber preference-link token (Phase 6 US2). Stored as a hash so a
-- database leak yields no usable links; unique per tenant so a token resolves
-- to exactly one subscriber.
ALTER TABLE subscribers ADD COLUMN preference_token_hash text;
CREATE UNIQUE INDEX subscribers_preference_token_idx
    ON subscribers (tenant_id, preference_token_hash)
    WHERE preference_token_hash IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON subscription_pages, pending_subscriptions TO nvelope_app;
