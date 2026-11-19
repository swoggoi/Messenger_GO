-- Академические группы пользователей. Это НЕ групповые чаты.
CREATE TABLE IF NOT EXISTS academic_groups (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name VARCHAR(20) UNIQUE NOT NULL
);

INSERT INTO academic_groups (name) VALUES
    ('Т-491'),
    ('Т-492'),
    ('Т-493'),
    ('Т-494'),
    ('Т-495')
ON CONFLICT (name) DO NOTHING;

-- Существующие пользователи остаются без группы (NULL).
ALTER TABLE users ADD COLUMN IF NOT EXISTS group_id UUID;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'users_group_id_fkey'
    ) THEN
        ALTER TABLE users
            ADD CONSTRAINT users_group_id_fkey
            FOREIGN KEY (group_id) REFERENCES academic_groups(id) ON DELETE SET NULL;
    END IF;
END
$$;

CREATE INDEX IF NOT EXISTS idx_users_group_id ON users(group_id);

-- Избранное создаётся один раз на пользователя; защищаем от гонки двух запросов.
CREATE UNIQUE INDEX IF NOT EXISTS idx_chats_saved_owner ON chats(created_by) WHERE type = 'saved';
