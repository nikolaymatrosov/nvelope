-- raw_html_sanitization is how a tenant's raw (non-visual) campaign and
-- template HTML is treated on save: 'standard' strips active content, 'off'
-- stores it verbatim (only after the tenant explicitly accepted the risk).
ALTER TABLE tenant_settings
    ADD COLUMN raw_html_sanitization text NOT NULL DEFAULT 'standard'
        CHECK (raw_html_sanitization IN ('standard', 'off'));
