-- The sending-domain tenant-plane table. A tenant registers a From domain,
-- receives DKIM/SPF/DMARC DNS records, and the platform polls until the domain
-- reaches verified or failed. Protected by Row-Level Security, reusing the
-- tenant_isolation pattern from 000004.

CREATE TABLE sending_domains (
    id                   uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id            uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    domain               text        NOT NULL,
    status               text        NOT NULL DEFAULT 'pending'
                                      CHECK (status IN ('pending', 'verified', 'failed')),
    dkim_records         jsonb       NOT NULL DEFAULT '[]',
    spf_record           text        NOT NULL DEFAULT '',
    dmarc_record         text        NOT NULL DEFAULT '',
    postbox_identity_ref text        NOT NULL DEFAULT '',
    failure_reason       text        NOT NULL DEFAULT '',
    created_at           timestamptz NOT NULL DEFAULT now(),
    verified_at          timestamptz,
    last_checked_at      timestamptz,
    UNIQUE (tenant_id, domain)
);

ALTER TABLE sending_domains ENABLE ROW LEVEL SECURITY;
ALTER TABLE sending_domains FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON sending_domains
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

CREATE INDEX sending_domains_tenant_status_idx ON sending_domains (tenant_id, status);

GRANT SELECT, INSERT, UPDATE, DELETE ON sending_domains TO nvelope_app;
