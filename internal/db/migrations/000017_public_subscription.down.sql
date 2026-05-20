DROP INDEX IF EXISTS subscribers_preference_token_idx;
ALTER TABLE subscribers DROP COLUMN IF EXISTS preference_token_hash;
DROP TABLE IF EXISTS pending_subscriptions;
DROP TABLE IF EXISTS subscription_pages;
