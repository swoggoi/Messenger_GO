package domain

import "time"

type Message struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	UserID    string    `json:"user_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

func NewMessage(id, chatID, userID, content string) *Message {
	return &Message{
		ID:        id,
		ChatID:    chatID,
		UserID:    userID,
		Content:   content,
		CreatedAt: time.Now(),
	}
}
