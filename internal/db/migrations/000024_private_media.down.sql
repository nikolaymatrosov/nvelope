ALTER TABLE media_assets
    DROP COLUMN published_at,
    DROP COLUMN height,
    DROP COLUMN width,
    ADD COLUMN public_url text NOT NULL DEFAULT '';
