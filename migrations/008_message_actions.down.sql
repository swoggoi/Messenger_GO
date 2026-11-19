DROP INDEX IF EXISTS idx_messages_pinned;
DROP INDEX IF EXISTS idx_messages_reply_to;
ALTER TABLE messages DROP COLUMN IF EXISTS is_pinned;
ALTER TABLE messages DROP COLUMN IF EXISTS reply_to_id;
