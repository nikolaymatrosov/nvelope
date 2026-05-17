-- The audience tenant-plane tables: lists, subscribers, and the membership
-- link between them. Every table carries tenant_id and is protected by
-- Row-Level Security, reusing the tenant_isolation pattern from 000004.

CREATE TABLE lists (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    name        text        NOT NULL,
    description text        NOT NULL DEFAULT '',
    visibility  text        NOT NULL DEFAULT 'private'
                            CHECK (visibility IN ('public', 'private')),
    optin       text        NOT NULL DEFAULT 'single'
                            CHECK (optin IN ('single', 'double')),
    tags        text[]      NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, name)
);

ALTER TABLE lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE lists FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON lists
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE TABLE subscribers (
    id          uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    email       citext      NOT NULL,
    name        text        NOT NULL DEFAULT '',
    state       text        NOT NULL DEFAULT 'enabled'
                            CHECK (state IN ('enabled', 'disabled', 'blocklisted')),
    attributes  jsonb       NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (tenant_id, email)
);

ALTER TABLE subscribers ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscribers FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscribers
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- GIN index over attributes so segment queries can filter custom attributes.
CREATE INDEX subscribers_attributes_idx ON subscribers USING gin (attributes);

CREATE TABLE subscriber_lists (
    tenant_id           uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    subscriber_id       uuid        NOT NULL REFERENCES subscribers (id) ON DELETE CASCADE,
    list_id             uuid        NOT NULL REFERENCES lists (id) ON DELETE CASCADE,
    subscription_status text        NOT NULL DEFAULT 'unconfirmed'
                            CHECK (subscription_status IN ('unconfirmed', 'confirmed', 'unsubscribed')),
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (subscriber_id, list_id)
);

ALTER TABLE subscriber_lists ENABLE ROW LEVEL SECURITY;
ALTER TABLE subscriber_lists FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON subscriber_lists
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

-- The deferred foreign key from user_list_roles (created in 000005) to lists,
-- added now that the lists table exists.
ALTER TABLE user_list_roles
    ADD CONSTRAINT user_list_roles_list_id_fkey
    FOREIGN KEY (list_id) REFERENCES lists (id) ON DELETE CASCADE;

GRANT SELECT, INSERT, UPDATE, DELETE ON lists, subscribers, subscriber_lists TO nvelope_app;
