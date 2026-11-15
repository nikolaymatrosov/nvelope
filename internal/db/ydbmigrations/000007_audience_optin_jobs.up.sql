-- Audience public subscription and bulk jobs (internal/audience): the public
-- subscription pages, the double-opt-in pending subscriptions they create, and
-- the import/export job records. Every table is tenant-scoped: its primary key
-- starts with tenant_id and every query filters by it. No column has a
-- DEFAULT, so the adapters write the values Postgres takes from its column
-- defaults. YDB has no foreign keys and no array type: list ids are a JSON
-- array in a Utf8 column.

-- slug is stored lower-cased (domain.NewSubscriptionPage lower-cases it), so
-- the unique index matches the Postgres UNIQUE (tenant_id, slug). fields is a
-- JSON array that is only stored and read back whole.
CREATE TABLE IF NOT EXISTS subscription_pages (
    tenant_id         Utf8      NOT NULL,
    id                Utf8      NOT NULL,
    slug              Utf8      NOT NULL,
    title             Utf8      NOT NULL,
    target_list_ids   Utf8      NOT NULL,
    fields            Utf8      NOT NULL,
    sending_domain_id Utf8      NOT NULL,
    from_name         Utf8      NOT NULL,
    from_local_part   Utf8      NOT NULL,
    active            Bool      NOT NULL,
    created_at        Timestamp NOT NULL,
    updated_at        Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_subscription_pages_slug GLOBAL UNIQUE SYNC ON (tenant_id, slug)
);

-- email is stored lower-cased (domain.NewPendingSubscription lower-cases it),
-- so the unique index reproduces the Postgres citext UNIQUE (tenant_id, email,
-- subscription_page_id) that Upsert targets. The token index serves the
-- confirmation-link lookup. attributes is only stored and read back whole.
CREATE TABLE IF NOT EXISTS pending_subscriptions (
    tenant_id               Utf8      NOT NULL,
    id                      Utf8      NOT NULL,
    subscription_page_id    Utf8      NOT NULL,
    email                   Utf8      NOT NULL,
    attributes              Utf8      NOT NULL,
    target_list_ids         Utf8      NOT NULL,
    confirmation_token_hash Utf8      NOT NULL,
    expires_at              Timestamp NOT NULL,
    created_at              Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_pending_subscriptions_email GLOBAL UNIQUE SYNC ON (tenant_id, email, subscription_page_id),
    INDEX idx_pending_subscriptions_token GLOBAL UNIQUE SYNC ON (tenant_id, confirmation_token_hash)
);

-- The staged upload (import) or generated file (export) lives in Object
-- Storage, not in the row: a YDB cell holds at most 16 MiB and an upload may
-- be 32 MB. file_key is NULL until the job has a file. params and failures are
-- JSON only stored and read back whole.
CREATE TABLE IF NOT EXISTS import_export_jobs (
    tenant_id     Utf8      NOT NULL,
    id            Utf8      NOT NULL,
    kind          Utf8      NOT NULL,
    requested_by  Utf8      NOT NULL,
    status        Utf8      NOT NULL,
    params        Utf8      NOT NULL,
    file_name     Utf8      NOT NULL,
    file_key      Utf8,
    created_count Int64     NOT NULL,
    updated_count Int64     NOT NULL,
    failed_count  Int64     NOT NULL,
    row_count     Int64     NOT NULL,
    failures      Utf8      NOT NULL,
    created_at    Timestamp NOT NULL,
    started_at    Timestamp,
    finished_at   Timestamp,
    PRIMARY KEY (tenant_id, id)
);
