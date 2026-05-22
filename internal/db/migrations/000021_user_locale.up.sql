-- Per-user interface-language preference for app internationalization.
--
-- `locale` is a personal, control-plane account attribute — which language the
-- signed-in user sees the UI in. It is nullable with no default so NULL
-- distinguishes "never explicitly chosen" from a deliberate choice of English;
-- that distinction drives the adopt-on-sign-in rule. platform_users is
-- control-plane, so no Row-Level Security. The CHECK is defence in depth — the
-- Locale value object is the primary guard.

ALTER TABLE platform_users
    ADD COLUMN locale text
    CHECK (locale IS NULL OR locale IN ('en', 'ru'));
