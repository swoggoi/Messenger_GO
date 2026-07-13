package domain

import "time"

type ChatMember struct {
	ChatID   string    `json:"chat_id"`
	UserID   string    `json:"user_id"`
	RoleID   string    `json:"role_id"`
	JoinedAt time.Time `json:"joined_at"`
}

func NewChatMember(chatID string, userID string, roleID string) *ChatMember {
	return &ChatMember{
		ChatID:   chatID,
		UserID:   userID,
		RoleID:   roleID,
		JoinedAt: time.Now(),
	}
}
