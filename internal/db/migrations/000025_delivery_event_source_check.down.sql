-- NOT VALID: events whose source was deleted have both ids NULL and would fail
-- the restored check; only new rows are checked.
ALTER TABLE delivery_events DROP CONSTRAINT delivery_events_source_check;
ALTER TABLE delivery_events ADD CONSTRAINT delivery_events_check
    CHECK ((campaign_recipient_id IS NOT NULL) <> (transactional_message_id IS NOT NULL)) NOT VALID;
