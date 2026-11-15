-- Media context (internal/media): the metadata of a tenant's private media
-- library. The bytes live in Object Storage under media/<tenant_id>/<id>/...;
-- this table holds only the row. No column has a DEFAULT, no CHECK and no
-- foreign key: the adapter writes every value and the domain keeps them valid
-- (see 000008_campaign). width and height are 0 for a non-image (Postgres
-- stores NULL), uploaded_by is NULL when unknown. The listing reads every row
-- of one tenant's key range and orders it in YQL, so no secondary index.
CREATE TABLE IF NOT EXISTS media_assets (
    tenant_id    Utf8      NOT NULL,
    id           Utf8      NOT NULL,
    filename     Utf8      NOT NULL,
    content_type Utf8      NOT NULL,
    size_bytes   Int64     NOT NULL,
    width        Int32     NOT NULL,
    height       Int32     NOT NULL,
    storage_key  Utf8      NOT NULL,
    uploaded_by  Utf8,
    published_at Timestamp,
    created_at   Timestamp NOT NULL,
    PRIMARY KEY (tenant_id, id)
);
