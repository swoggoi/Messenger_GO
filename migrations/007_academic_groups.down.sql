DROP INDEX IF EXISTS idx_chats_saved_owner;
ALTER TABLE users DROP CONSTRAINT IF EXISTS users_group_id_fkey;
ALTER TABLE users DROP COLUMN IF EXISTS group_id;
DROP TABLE IF EXISTS academic_groups;
