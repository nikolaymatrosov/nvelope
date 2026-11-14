-- Sending domains (internal/sending). Tenant-scoped: every query filters by
-- tenant_id, and the primary key starts with it so that filter is a key-prefix
-- lookup. dkim_records is JSON that is only stored and read back whole, so it
-- is a plain Utf8 column.
CREATE TABLE IF NOT EXISTS sending_domains (
    tenant_id            Utf8      NOT NULL,
    id                   Utf8      NOT NULL,
    domain               Utf8      NOT NULL,
    status               Utf8      NOT NULL,
    dkim_records         Utf8      NOT NULL,
    spf_record           Utf8      NOT NULL,
    dmarc_record         Utf8      NOT NULL,
    postbox_identity_ref Utf8      NOT NULL,
    failure_reason       Utf8      NOT NULL,
    created_at           Timestamp NOT NULL,
    verified_at          Timestamp,
    last_checked_at      Timestamp,
    PRIMARY KEY (tenant_id, id),
    INDEX idx_sending_domains_tenant_domain GLOBAL UNIQUE SYNC ON (tenant_id, domain)
);
