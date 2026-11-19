package messages

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

// Store is the persistence contract used by the message service.
type Store interface {
	CreateMessage(ctx context.Context, msg *domain.Message) error
	GetMessages(ctx context.Context, chatID uuid.UUID, limit, offset int) ([]domain.Message, error)
	GetMessageByID(ctx context.Context, messageID string) (*domain.Message, error)
	DeleteMessage(ctx context.Context, messageID string) error
	SetMessagePinned(ctx context.Context, messageID string, pinned bool) error
	IsChatMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error)
	GetChatMemberIDs(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error)
	MarkChatAsRead(ctx context.Context, chatID, userID uuid.UUID) error
	MarkChatAsUnread(ctx context.Context, chatID, userID uuid.UUID) error
	DeliverPendingMessages(ctx context.Context, chatID, userID uuid.UUID) error
}

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateMessage(ctx context.Context, msg *domain.Message) error {
	query := `
		INSERT INTO messages (chat_id, sender_id, content, status, reply_to_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at,
			COALESCE((SELECT COALESCE(NULLIF(u.name, ''), u.username, '') FROM users u WHERE u.id = $2), '')
	`
	return r.pool.QueryRow(ctx, query, msg.ChatID, msg.SenderID, msg.Content, msg.Status, msg.ReplyToID).
		Scan(&msg.ID, &msg.CreatedAt, &msg.SenderName)
}

func (r *Repository) GetMessages(ctx context.Context, chatID uuid.UUID, limit, offset int) ([]domain.Message, error) {
	query := `
		SELECT m.id, m.chat_id, m.sender_id,
		       COALESCE(NULLIF(u.name, ''), u.username, ''), m.content, m.status,
		       m.reply_to_id, m.is_pinned, m.edited_at, m.deleted_at, m.created_at
		FROM messages m
		INNER JOIN users u ON m.sender_id = u.id
		WHERE m.chat_id = $1 AND m.deleted_at IS NULL
		ORDER BY m.created_at ASC, m.id ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	messages := []domain.Message{}
	for rows.Next() {
		var msg domain.Message
		var editedAt, deletedAt *time.Time
		if err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.SenderName,
			&msg.Content, &msg.Status, &msg.ReplyToID, &msg.IsPinned, &editedAt, &deletedAt, &msg.CreatedAt); err != nil {
			return nil, err
		}
		msg.EditedAt = editedAt
		msg.DeletedAt = deletedAt
		messages = append(messages, msg)
	}
	return messages, rows.Err()
}

func (r *Repository) IsChatMember(ctx context.Context, chatID, userID uuid.UUID) (bool, error) {
	var exists bool
	err := r.pool.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`,
		chatID, userID,
	).Scan(&exists)
	return exists, err
}

func (r *Repository) GetChatMemberIDs(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error) {
	rows, err := r.pool.Query(ctx, `SELECT user_id FROM chat_members WHERE chat_id = $1`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) MarkChatAsRead(ctx context.Context, chatID, userID uuid.UUID) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE messages SET status = 'read'
		WHERE chat_id = $1 AND sender_id != $2 AND status IN ('sent', 'delivered')
	`, chatID, userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE chat_members SET last_read_at = CURRENT_TIMESTAMP WHERE chat_id = $1 AND user_id = $2
	`, chatID, userID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) MarkChatAsUnread(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE messages SET status = 'delivered'
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'read'
	`, chatID, userID)
	return err
}

func (r *Repository) DeliverPendingMessages(ctx context.Context, chatID, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `
		UPDATE messages SET status = 'delivered'
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'sent'
	`, chatID, userID)
	return err
}

// The methods below are kept for future moderation/editing endpoints.

func (r *Repository) GetMessageByID(ctx context.Context, messageID string) (*domain.Message, error) {
	query := `
		SELECT m.id, m.chat_id, m.sender_id,
		       COALESCE(NULLIF(u.name, ''), u.username, ''), m.content, m.status,
		       m.reply_to_id, m.is_pinned, m.edited_at, m.deleted_at, m.created_at
		FROM messages m INNER JOIN users u ON m.sender_id = u.id
		WHERE m.id = $1
	`
	msg := &domain.Message{}
	var editedAt, deletedAt *time.Time
	err := r.pool.QueryRow(ctx, query, messageID).Scan(&msg.ID, &msg.ChatID, &msg.SenderID,
		&msg.SenderName, &msg.Content, &msg.Status, &msg.ReplyToID, &msg.IsPinned,
		&editedAt, &deletedAt, &msg.CreatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core_errors.ErrMessageNotFound
		}
		return nil, err
	}
	if msg.DeletedAt != nil {
		return nil, core_errors.ErrMessageNotFound
	}
	return msg, nil
}

func (r *Repository) DeleteMessage(ctx context.Context, messageID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE messages SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1`, messageID)
	return err
}

func (r *Repository) SetMessagePinned(ctx context.Context, messageID string, pinned bool) error {
	_, err := r.pool.Exec(ctx, `UPDATE messages SET is_pinned = $1 WHERE id = $2`, pinned, messageID)
	return err
}

func (r *Repository) EditMessage(ctx context.Context, messageID, content string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE messages SET content = $1, edited_at = CURRENT_TIMESTAMP WHERE id = $2`, content, messageID)
	return err
}

func (r *Repository) UpdateMessageStatus(ctx context.Context, messageID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx, `UPDATE messages SET status = $1 WHERE id = $2`, status, messageID)
	return err
}
