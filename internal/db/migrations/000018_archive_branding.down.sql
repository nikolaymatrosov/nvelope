DROP TABLE IF EXISTS tenant_branding;
DROP INDEX IF EXISTS campaigns_archive_idx;
ALTER TABLE campaigns DROP COLUMN IF EXISTS archived_at;
ALTER TABLE campaigns DROP COLUMN IF EXISTS archive_visible;
