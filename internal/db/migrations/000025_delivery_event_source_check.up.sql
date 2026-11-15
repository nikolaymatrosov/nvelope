-- A delivery event outlives its source. campaign_recipient_id and
-- transactional_message_id are ON DELETE SET NULL, but the original CHECK
-- demanded exactly one of them, so the SET NULL could never succeed: deleting
-- a subscriber (which cascades to its campaign_recipients) failed whenever one
-- of its recipients had a delivery event. An event now names at most one
-- source; NULL in both means its source was deleted.
ALTER TABLE delivery_events DROP CONSTRAINT delivery_events_check;
ALTER TABLE delivery_events ADD CONSTRAINT delivery_events_source_check
    CHECK (campaign_recipient_id IS NULL OR transactional_message_id IS NULL);
