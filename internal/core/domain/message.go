package domain

import (
	"time"

	"github.com/google/uuid"
)

type Message struct {
	ID           uuid.UUID `json:"id"`
	ChatID       uuid.UUID `json:"chat_id"`
	SenderID     uuid.UUID `json:"sender_id"`
	Content      string    `json:"content"`
	ClientTempID string    `json:"client_temp_id,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type SendMessageRequest struct {
	ChatID       uuid.UUID `json:"chat_id" validate:"required"`
	Content      string    `json:"content" validate:"required,max=4000"`
	ClientTempID string    `json:"client_temp_id,omitempty"`
}
