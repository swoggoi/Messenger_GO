package chats

import (
	"context"
	"errors"
	"time"

	"messenger/internal/core/domain"
	"messenger/internal/db"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Store keeps chat business rules testable without PostgreSQL.
type Store interface {
	CreateChat(ctx context.Context, chat *domain.Chat) error
	CreatePrivateChat(ctx context.Context, userID, otherUserID uuid.UUID, chat *domain.Chat) error
	AddMember(ctx context.Context, chatID, userID uuid.UUID, role string) error
	FindPrivateChat(ctx context.Context, userID, otherUserID uuid.UUID) (*domain.Chat, error)
	FindSavedChat(ctx context.Context, userID uuid.UUID) (*domain.Chat, error)
	GetChatByID(ctx context.Context, chatID uuid.UUID) (*domain.Chat, error)
	GetMyChats(ctx context.Context, userID uuid.UUID) ([]domain.ChatListItem, error)
	GetChatsByUser(ctx context.Context, userID uuid.UUID) ([]domain.Chat, error)
	GetChatMembers(ctx context.Context, chatID uuid.UUID) ([]domain.ChatMember, error)
	IsMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error)
	RemoveMember(ctx context.Context, chatID, userID uuid.UUID) error
	GetMemberRole(ctx context.Context, chatID, userID uuid.UUID) (string, error)
}

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateChat(ctx context.Context, chat *domain.Chat) error {
	query := `INSERT INTO chats (type, name, created_by) VALUES ($1, NULLIF($2, ''), $3) RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, query, chat.Type, chat.Name, chat.CreatedBy).Scan(&chat.ID, &chat.CreatedAt)
	return err
}

// CreatePrivateChat reuses an existing private chat even under concurrent
// requests: both participants are locked in a deterministic order first.
func (r *Repository) CreatePrivateChat(ctx context.Context, userID, otherUserID uuid.UUID, chat *domain.Chat) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	first, second := userID.String(), otherUserID.String()
	if first > second {
		first, second = second, first
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, first+"|"+second); err != nil {
		return err
	}

	existing, err := findPrivateChat(ctx, tx, userID, otherUserID)
	if err != nil {
		return err
	}
	if existing != nil {
		*chat = *existing
		return tx.Commit(ctx)
	}

	query := `INSERT INTO chats (type, created_by) VALUES ('private', $1) RETURNING id, created_at`
	if err := tx.QueryRow(ctx, query, userID).Scan(&chat.ID, &chat.CreatedAt); err != nil {
		return err
	}
	chat.Type = "private"
	if _, err := tx.Exec(ctx, `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'owner')`, chat.ID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, 'member')`, chat.ID, otherUserID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type rowQuerier interface {
	QueryRow(ctx context.Context, sql string, args ...interface{}) pgx.Row
}

func findPrivateChat(ctx context.Context, q rowQuerier, userID, otherUserID uuid.UUID) (*domain.Chat, error) {
	query := `
		SELECT c.id, c.type, COALESCE(c.name, ''), COALESCE(c.avatar_url, ''), c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm1 ON c.id = cm1.chat_id AND cm1.user_id = $1
		INNER JOIN chat_members cm2 ON c.id = cm2.chat_id AND cm2.user_id = $2
		WHERE c.type = 'private'
		ORDER BY c.created_at
		LIMIT 1
	`
	chat := &domain.Chat{}
	err := q.QueryRow(ctx, query, userID, otherUserID).Scan(
		&chat.ID, &chat.Type, &chat.Name, &chat.AvatarURL, &chat.CreatedBy, &chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return chat, nil
}

func (r *Repository) FindPrivateChat(ctx context.Context, userID, otherUserID uuid.UUID) (*domain.Chat, error) {
	return findPrivateChat(ctx, r.pool, userID, otherUserID)
}

func (r *Repository) FindSavedChat(ctx context.Context, userID uuid.UUID) (*domain.Chat, error) {
	query := `
		SELECT c.id, c.type, COALESCE(c.name, ''), COALESCE(c.avatar_url, ''), c.created_by, c.created_at
		FROM chats c
		INNER JOIN chat_members cm ON c.id = cm.chat_id AND cm.user_id = $1
		WHERE c.type = 'saved'
		ORDER BY c.created_at
		LIMIT 1
	`
	chat := &domain.Chat{}
	err := r.pool.QueryRow(ctx, query, userID).Scan(
		&chat.ID, &chat.Type, &chat.Name, &chat.AvatarURL, &chat.CreatedBy, &chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	return chat, nil
}

func (r *Repository) AddMember(ctx context.Context, chatID, userID uuid.UUID, role string) error {
	query := `INSERT INTO chat_members (chat_id, user_id, role) VALUES ($1, $2, $3)`
	_, err := r.pool.Exec(ctx, query, chatID, userID, role)
	return err
}

func (r *Repository) GetChatByID(ctx context.Context, chatID uuid.UUID) (*domain.Chat, error) {
	query := `SELECT id, type, COALESCE(name, ''), COALESCE(avatar_url, ''), created_by, created_at FROM chats WHERE id = $1`
	chat := &domain.Chat{}
	err := r.pool.QueryRow(ctx, query, chatID).Scan(
		&chat.ID, &chat.Type, &chat.Name, &chat.AvatarURL, &chat.CreatedBy, &chat.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core_errors.ErrChatNotFound
		}
		return nil, err
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
					WHEN c.type = 'private' THEN COALESCE(NULLIF(pu.name, ''), pu.username, 'Участник')
					ELSE NULLIF(c.name, '')
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
			ORDER BY pcm.joined_at
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

	chats := []domain.ChatListItem{}
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
		item.PartnerID = partnerID
		item.LastSenderID = lastSenderID
		item.LastMessageTime = lastMessageTime
		if lastMessage != nil {
			item.LastMessage = *lastMessage
		}
		if lastStatus != nil {
			item.LastStatus = *lastStatus
		}
		chats = append(chats, item)
	}
	return chats, rows.Err()
}

func (r *Repository) GetChatsByUser(ctx context.Context, userID uuid.UUID) ([]domain.Chat, error) {
	query := `
		SELECT c.id, c.type, COALESCE(c.name, ''), COALESCE(c.created_by, '00000000-0000-0000-0000-000000000000'::uuid), c.created_at
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

	chats := []domain.Chat{}
	for rows.Next() {
		var chat domain.Chat
		if err := rows.Scan(&chat.ID, &chat.Type, &chat.Name, &chat.CreatedBy, &chat.CreatedAt); err != nil {
			return nil, err
		}
		chats = append(chats, chat)
	}
	return chats, rows.Err()
}

func (r *Repository) GetChatMembers(ctx context.Context, chatID uuid.UUID) ([]domain.ChatMember, error) {
	query := `SELECT chat_id, user_id, role, joined_at, last_read_at FROM chat_members WHERE chat_id = $1`
	rows, err := r.pool.Query(ctx, query, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	members := []domain.ChatMember{}
	for rows.Next() {
		var member domain.ChatMember
		var lastReadAt *time.Time
		if err := rows.Scan(&member.ChatID, &member.UserID, &member.Role, &member.JoinedAt, &lastReadAt); err != nil {
			return nil, err
		}
		if lastReadAt != nil {
			member.LastReadAt = *lastReadAt
		}
		members = append(members, member)
	}
	return members, rows.Err()
}

func (r *Repository) IsMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`,
		chatID, userID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) RemoveMember(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `DELETE FROM chat_members WHERE chat_id = $1 AND user_id = $2`, chatID, userID)
	return err
}

func (r *Repository) GetMemberRole(ctx context.Context, chatID, userID uuid.UUID) (string, error) {
	var role string
	err := r.pool.QueryRow(ctx,
		`SELECT role FROM chat_members WHERE chat_id = $1 AND user_id = $2`, chatID, userID,
	).Scan(&role)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", core_errors.ErrForbidden
		}
		return "", err
	}
	return role, nil
}
