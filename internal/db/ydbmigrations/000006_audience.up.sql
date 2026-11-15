-- Audience core (internal/audience): lists, subscribers, list memberships and
-- the subscriber custom-field registry. Every table is tenant-scoped: its
-- primary key starts with tenant_id and every query filters by it. No column
-- has a DEFAULT, so the adapters write the values Postgres takes from its
-- column defaults. YDB has no foreign keys: the adapters check a membership's
-- subscriber and list, and delete the rows Postgres would cascade.

-- tags is a JSON array of strings, only stored and read back whole, so it is a
-- plain Utf8 column (Postgres: text[]). List names are case-sensitive, as the
-- Postgres text column is.
CREATE TABLE IF NOT EXISTS lists (
    tenant_id   Utf8      NOT NULL,
    id          Utf8      NOT NULL,
    name        Utf8      NOT NULL,
    description Utf8      NOT NULL,
    visibility  Utf8      NOT NULL,
    optin       Utf8      NOT NULL,
    tags        Utf8      NOT NULL,
    created_at  Timestamp NOT NULL,
    updated_at  Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_lists_tenant_name GLOBAL UNIQUE SYNC ON (tenant_id, name)
);

-- email is stored lower-cased (domain.NewSubscriber lower-cases it), so the
-- unique index reproduces the uniqueness of the Postgres citext column.
-- attributes is a JsonDocument because segment queries read into it with
-- JSON_VALUE and JSON_EXISTS.
CREATE TABLE IF NOT EXISTS subscribers (
    tenant_id  Utf8         NOT NULL,
    id         Utf8         NOT NULL,
    email      Utf8         NOT NULL,
    name       Utf8         NOT NULL,
    state      Utf8         NOT NULL,
    attributes JsonDocument NOT NULL,
    created_at Timestamp    NOT NULL,
    updated_at Timestamp    NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscribers_email GLOBAL UNIQUE SYNC ON (tenant_id, email)
);

-- Keyed by subscriber, so a subscriber's memberships are a key-prefix read.
-- The list index serves InList, list-membership segment conditions and the
-- cascade when a list is deleted.
CREATE TABLE IF NOT EXISTS subscriber_lists (
    tenant_id           Utf8      NOT NULL,
    subscriber_id       Utf8      NOT NULL,
    list_id             Utf8      NOT NULL,
    subscription_status Utf8      NOT NULL,
    created_at          Timestamp NOT NULL,
    updated_at          Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, subscriber_id, list_id),
    INDEX idx_subscriber_lists_list GLOBAL SYNC ON (tenant_id, list_id)
);

CREATE TABLE IF NOT EXISTS subscriber_fields (
    tenant_id     Utf8      NOT NULL,
    id            Utf8      NOT NULL,
    slug          Utf8      NOT NULL,
    display_name  Utf8      NOT NULL,
    type          Utf8      NOT NULL,
    default_value Utf8      NOT NULL,
    position      Int64     NOT NULL,
    created_at    Timestamp NOT NULL,
    updated_at    Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscriber_fields_slug GLOBAL UNIQUE SYNC ON (tenant_id, slug)
);
