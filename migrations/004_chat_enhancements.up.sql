ALTER TABLE users ADD COLUMN IF NOT EXISTS name VARCHAR(100);
UPDATE users SET name = username WHERE name IS NULL OR name = '';

ALTER TABLE chat_members ADD COLUMN IF NOT EXISTS last_read_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_chat_members_last_read ON chat_members(chat_id, user_id, last_read_at);
