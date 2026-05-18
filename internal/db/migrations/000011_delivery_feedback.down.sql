DROP FUNCTION IF EXISTS feedback_tenant_for_message(text);

DROP INDEX IF EXISTS campaign_recipients_provider_message_id_idx;

ALTER TABLE campaign_recipients DROP CONSTRAINT campaign_recipients_status_check;
ALTER TABLE campaign_recipients ADD CONSTRAINT campaign_recipients_status_check
    CHECK (status IN ('pending', 'sent', 'failed'));

ALTER TABLE campaign_recipients DROP COLUMN IF EXISTS provider_message_id;

DROP TABLE IF EXISTS delivery_events;
DROP TABLE IF EXISTS transactional_messages;
DROP TABLE IF EXISTS inbound_feedback_events;
