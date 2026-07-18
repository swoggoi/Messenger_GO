package messages

import (
	"context"
	"errors"
	"time"

	"messenger/internal/core/domain"
	"messenger/internal/db"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateMessage(ctx context.Context, msg *domain.Message) error {
	query := `
		INSERT INTO messages (chat_id, sender_id, content, status) 
		VALUES ($1, $2, $3, $4) 
		RETURNING id, created_at
	`
	err := r.pool.QueryRow(ctx, query, msg.ChatID, msg.SenderID, msg.Content, msg.Status).
		Scan(&msg.ID, &msg.CreatedAt)
	return err
}

func (r *Repository) GetMessages(ctx context.Context, chatID string, limit int, offset int) ([]domain.Message, error) {
	query := `
		SELECT m.id, m.chat_id, m.sender_id, COALESCE(u.name, u.username), m.content, m.status, 
		       m.edited_at, m.deleted_at, m.created_at
		FROM messages m
		INNER JOIN users u ON m.sender_id = u.id
		WHERE m.chat_id = $1 AND m.deleted_at IS NULL
		ORDER BY m.created_at ASC
		LIMIT $2 OFFSET $3
	`
	rows, err := r.pool.Query(ctx, query, chatID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []domain.Message
	for rows.Next() {
		var msg domain.Message
		var editedAt, deletedAt *time.Time

		err := rows.Scan(&msg.ID, &msg.ChatID, &msg.SenderID, &msg.SenderName,
			&msg.Content, &msg.Status, &editedAt, &deletedAt, &msg.CreatedAt)
		if err != nil {
			return nil, err
		}

		messages = append(messages, msg)
	}

	return messages, rows.Err()
}

func (r *Repository) MarkChatAsRead(ctx context.Context, chatID, userID string) error {
	query := `
		UPDATE messages 
		SET status = 'read' 
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'delivered'
	`
	_, err := r.pool.Exec(ctx, query, chatID, userID)
	return err
}
func (r *Repository) MarkChatAsUnread(ctx context.Context, chatID, userID string) error {
	query := `
		UPDATE messages 
		SET status = 'delivered' 
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'read'
	`
	_, err := r.pool.Exec(ctx, query, chatID, userID)
	return err
}

func (r *Repository) MarkMessagesDelivered(ctx context.Context, chatID, recipientID string) ([]domain.MessageStatusPayload, error) {
	query := `
		UPDATE messages
		SET status = 'delivered'
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'sent'
		RETURNING id, chat_id, status
	`
	rows, err := r.pool.Query(ctx, query, chatID, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var updates []domain.MessageStatusPayload
	for rows.Next() {
		var payload domain.MessageStatusPayload
		if err := rows.Scan(&payload.MessageID, &payload.ChatID, &payload.Status); err != nil {
			return nil, err
		}
		updates = append(updates, payload)
	}
	return updates, rows.Err()
}
func (r *Repository) DeliverPendingMessages(ctx context.Context, chatID, userID string) error {
	query := `
		SELECT id, chat_id, sender_id, content, status, created_at 
		FROM messages 
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'sent'
		ORDER BY created_at ASC
	`

	rows, err := r.pool.Query(ctx, query, chatID, userID)
	if err != nil {
		return err
	}
	defer rows.Close()

	updateQuery := `
		UPDATE messages 
		SET status = 'delivered' 
		WHERE chat_id = $1 AND sender_id != $2 AND status = 'sent'
	`
	_, err = r.pool.Exec(ctx, updateQuery, chatID, userID)
	return err
}
func (r *Repository) GetMessageByID(ctx context.Context, messageID string) (*domain.Message, error) {
	query := `
		SELECT m.id, m.chat_id, m.sender_id, COALESCE(u.name, u.username), m.content, m.status, 
		       m.edited_at, m.deleted_at, m.created_at
		FROM messages m
		INNER JOIN users u ON m.sender_id = u.id
		WHERE m.id = $1
	`
	msg := &domain.Message{}
	var editedAt, deletedAt *time.Time

	err := r.pool.QueryRow(ctx, query, messageID).Scan(
		&msg.ID, &msg.ChatID, &msg.SenderID, &msg.SenderName,
		&msg.Content, &msg.Status, &editedAt, &deletedAt, &msg.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("message not found")
		}
		return nil, err
	}

	return msg, nil
}

func (r *Repository) DeleteMessage(ctx context.Context, messageID string) error {
	query := `UPDATE messages SET deleted_at = CURRENT_TIMESTAMP WHERE id = $1`
	_, err := r.pool.Exec(ctx, query, messageID)
	return err
}

func (r *Repository) EditMessage(ctx context.Context, messageID, content string) error {
	query := `UPDATE messages SET content = $1, edited_at = CURRENT_TIMESTAMP WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, content, messageID)
	return err
}

func (r *Repository) IsChatMember(ctx context.Context, chatID, userID string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM chat_members WHERE chat_id = $1 AND user_id = $2)`
	var exists bool
	err := r.pool.QueryRow(ctx, query, chatID, userID).Scan(&exists)
	return exists, err
}

func (r *Repository) GetChatMemberIDs(ctx context.Context, chatID uuid.UUID) ([]uuid.UUID, error) {
	query := `SELECT user_id FROM chat_members WHERE chat_id = $1`
	rows, err := r.pool.Query(ctx, query, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

func (r *Repository) UpdateMessageStatus(ctx context.Context, messageID uuid.UUID, status string) error {
	query := `UPDATE messages SET status = $1 WHERE id = $2`
	_, err := r.pool.Exec(ctx, query, status, messageID)
	return err
}
