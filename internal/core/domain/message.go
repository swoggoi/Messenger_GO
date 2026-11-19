package domain

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID         uuid.UUID  `json:"id"`
	ChatID     uuid.UUID  `json:"chat_id"`
	SenderID   uuid.UUID  `json:"sender_id"`
	SenderName string     `json:"sender_name"`
	Content    string     `json:"content"`
	Status     string     `json:"status"`
	ReplyToID  *uuid.UUID `json:"reply_to_id,omitempty"`
	IsPinned   bool       `json:"is_pinned"`
	EditedAt   *time.Time `json:"edited_at,omitempty"`
	DeletedAt  *time.Time `json:"deleted_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type WSEvent struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

type MessageStatusPayload struct {
	MessageID uuid.UUID `json:"message_id"`
	ChatID    uuid.UUID `json:"chat_id"`
	Status    string    `json:"status"`
}
