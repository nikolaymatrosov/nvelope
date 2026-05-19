-- Phase 6 US4: tenant media library. Metadata rows live here under Row-Level
-- Security; the bytes live in S3-compatible object storage. The composite
-- (tenant_id, created_at) index supports the library listing's newest-first
-- order without scanning other tenants' rows.

CREATE TABLE media_assets (
    id           uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid        NOT NULL REFERENCES tenants (id) ON DELETE CASCADE,
    filename     text        NOT NULL,
    content_type text        NOT NULL,
    size_bytes   bigint      NOT NULL CHECK (size_bytes > 0),
    storage_key  text        NOT NULL,
    public_url   text        NOT NULL,
    uploaded_by  uuid        REFERENCES users (id) ON DELETE SET NULL,
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX media_assets_tenant_created_idx
    ON media_assets (tenant_id, created_at DESC);

ALTER TABLE media_assets ENABLE ROW LEVEL SECURITY;
ALTER TABLE media_assets FORCE  ROW LEVEL SECURITY;
CREATE POLICY tenant_isolation ON media_assets
    USING      (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid)
    WITH CHECK (tenant_id = nullif(current_setting('app.tenant_id', true), '')::uuid);

GRANT SELECT, INSERT, UPDATE, DELETE ON media_assets TO nvelope_app;
