DROP TABLE IF EXISTS subscriber_fields;

ALTER TABLE campaigns
    DROP COLUMN IF EXISTS theme,
    DROP COLUMN IF EXISTS body_doc;

ALTER TABLE templates
    DROP COLUMN IF EXISTS theme,
    DROP COLUMN IF EXISTS body_doc;
