package chats

import (
	"context"
	"errors"
	"messenger/internal/core/domain"
	"messenger/internal/db"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateChat(ctx context.Context, chat *domain.Chat) error {
	query := `INSERT INTO chats (type, name, created_by) VALUES ($1, $2, $3) RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, query, chat.Type, chat.Name, chat.CreatedBy).Scan(&chat.ID, &chat.CreatedAt)
	return err
}

func (r *Repository) AddMember(ctx context.Context, chatID, userID uuid.UUID, role string) error {
	query := `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, query, chatID, userID, role)
	return err
}

func (r *Repository) FindPrivateChat(ctx context.Context, userID, otherUserID uuid.UUID) (*domain.Chat, error) {
	query := `
		SELECT c.id, c.type, c.name, c.avatar_url, c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm1 ON c.id = cm1.chat_id AND cm1.user_id = $1
		INNER JOIN chat_members cm2 ON c.id = cm2.chat_id AND cm2.user_id = $2
		WHERE c.type = 'private'
		LIMIT 1
	`
	chat := &domain.Chat{}
	var name, avatarURL *string
	err := r.pool.QueryRow(ctx, query, userID, otherUserID).Scan(
		&chat.ID, &chat.Type, &name, &avatarURL, &chat.CreatedBy, &chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if name != nil {
		chat.Name = *name
	}
	if avatarURL != nil {
		chat.AvatarURL = *avatarURL
	}
	return chat, nil
}

func (r *Repository) FindSavedChat(ctx context.Context, userID uuid.UUID) (*domain.Chat, error) {
	query := `
		SELECT c.id, c.type, c.name, c.avatar_url, c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm ON c.id = cm.chat_id AND cm.user_id = $1
		WHERE c.type = 'saved'
		LIMIT 1
	`
	chat := &domain.Chat{}
	var name, avatarURL *string
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&chat.ID, &chat.Type, &name, &avatarURL, &chat.CreatedBy, &chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if name != nil {
		chat.Name = *name
	}
	if avatarURL != nil {
		chat.AvatarURL = *avatarURL
	}
	return chat, nil
}

func (r *Repository) GetChatByID(ctx context.Context, chatID uuid.UUID) (*domain.Chat, error) {
	query := `SELECT id, type, name, avatar_url, created_by, created_at FROM chats WHERE id = $1`
	chat := &domain.Chat{}
	var name, avatarURL *string
	err := r.pool.QueryRow(ctx, query, chatID).Scan(&chat.ID, &chat.Type, &name, &avatarURL, &chat.CreatedBy, &chat.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("chat not found")
		}
		return nil, err
	}
	if name != nil {
		chat.Name = *name
	}
	if avatarURL != nil {
		chat.AvatarURL = *avatarURL
	}
	return chat, nil
}

func (r *Repository) GetMyChats(ctx context.Context, userID uuid.UUID) ([]domain.ChatListItem, error) {
	query := `
		SELECT
			c.id,
			c.type,
			COALESCE(
				CASE
					WHEN c.type = 'saved' THEN 'Избранное'
					WHEN c.type = 'private' THEN COALESCE(pu.name, pu.username)
					ELSE c.name
				END,
				'Чат'
			) AS display_name,
			CASE
				WHEN c.type = 'saved' THEN NULL
				WHEN c.type = 'private' THEN pu.avatar_url
				ELSE c.avatar_url
			END AS avatar_url,
			CASE WHEN c.type = 'private' THEN pu.id ELSE NULL END AS partner_id,
			lm.content,
			lm.created_at,
			lm.sender_id,
			lm.status,
			COALESCE(uc.unread_count, 0),
			c.created_at
		FROM chats c
		INNER JOIN chat_members cm ON c.id = cm.chat_id AND cm.user_id = $1
		LEFT JOIN LATERAL (
			SELECT u.id, u.name, u.username, u.avatar_url
			FROM chat_members pcm
			INNER JOIN users u ON u.id = pcm.user_id
			WHERE pcm.chat_id = c.id AND pcm.user_id != $1
			LIMIT 1
		) pu ON c.type = 'private'
		LEFT JOIN LATERAL (
			SELECT m.content, m.created_at, m.sender_id, m.status
			FROM messages m
			WHERE m.chat_id = c.id AND m.deleted_at IS NULL
			ORDER BY m.created_at DESC
			LIMIT 1
		) lm ON true
		LEFT JOIN LATERAL (
			SELECT COUNT(*)::int AS unread_count
			FROM messages m
			WHERE m.chat_id = c.id
				AND m.sender_id != $1
				AND m.deleted_at IS NULL
				AND m.created_at > COALESCE(cm.last_read_at, '1970-01-01'::timestamptz)
		) uc ON true
		ORDER BY CASE WHEN c.type = 'saved' THEN 0 ELSE 1 END, COALESCE(lm.created_at, c.created_at) DESC
	`
	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []domain.ChatListItem
	for rows.Next() {
		var item domain.ChatListItem
		var avatarURL, lastMessage, lastStatus *string
		var partnerID, lastSenderID *uuid.UUID
		var lastMessageTime *time.Time

		if err := rows.Scan(
			&item.ID, &item.Type, &item.Name, &avatarURL, &partnerID,
			&lastMessage, &lastMessageTime, &lastSenderID, &lastStatus,
			&item.UnreadCount, &item.CreatedAt,
		); err != nil {
			return nil, err
		}

		if avatarURL != nil {
			item.AvatarURL = *avatarURL
		}
		if partnerID != nil {
			item.PartnerID = partnerID
		}
		if lastMessage != nil {
			item.LastMessage = *lastMessage
		}
		if lastSenderID != nil {
			item.LastSenderID = lastSenderID
		}
		if lastStatus != nil {
			item.LastStatus = *lastStatus
		}
		if lastMessageTime != nil {
			item.LastMessageTime = lastMessageTime
		}

		chats = append(chats, item)
	}
	return chats, rows.Err()
}

func (r *Repository) GetChatsByUser(ctx context.Context, userID uuid.UUID) ([]domain.Chat, error) {
	query := `
        SELECT 
            c.id, 
            c.type, 
            COALESCE(c.name, ''), 
            COALESCE(c.created_by, '00000000-0000-0000-0000-000000000000'::uuid), 
            c.created_at
        FROM chats c
        JOIN chat_members cm ON c.id = cm.chat_id
        WHERE cm.user_id = $1
        ORDER BY c.created_at DESC
    `

	rows, err := r.pool.Query(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var chats []domain.Chat
	for rows.Next() {
		var chat domain.Chat
		if err := rows.Scan(&chat.ID, &chat.Type, &chat.Name, &chat.CreatedBy, &chat.CreatedAt); err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}

	return chats, nil
}

func (r *Repository) GetChatMembers(ctx context.Context, chatID uuid.UUID) ([]domain.ChatMember, error) {
	query := `SELECT chat_id, user_id, role, joined_at, last_read_at FROM chat_members WHERE chat_id = $1`
	rows, err := r.pool.Query(ctx, query, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []domain.ChatMember
	for rows.Next() {
		var member domain.ChatMember
		if err := rows.Scan(&member.ChatID, &member.UserID, &member.Role, &member.JoinedAt, &member.LastReadAt); err != nil {
			return nil, err
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *Repository) IsMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, chatID, userID).Scan(&exists)
	return exists, err
}

func (r *Repository) RemoveMember(ctx context.Context, chatID, userID uuid.UUID) error {
	query := `DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`
	_, err := r.pool.Exec(ctx, query, chatID, userID)
	return err
}

func (r *Repository) GetMemberRole(ctx context.Context, chatID, userID uuid.UUID) (string, error) {
	query := `SELECT role FROM chat_members WHERE chat_id = $1 AND user_id = $2`
	var role string
	err := r.pool.QueryRow(ctx, query, chatID, userID).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", errors.New("not a member")
		}
		return "", err
	}
	return role, nil
}
