package domain

import (
	"time"

	"github.com/google/uuid"
)

type User struct {
	ID           uuid.UUID `json:"id"`
	Username     string    `json:"username"`
	Name         string    `json:"name"`
	Email        string    `json:"email"`
	PasswordHash string    `json:"-"`
	AvatarURL    string    `json:"avatar_url,omitempty"`
	CreatedAt    time.Time `json:"created_at"`
}

type RegisterRequest struct {
	Username string `json:"username"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

type Chat struct {
	ID        uuid.UUID    `json:"id"`
	Type      string       `json:"type"`
	Name      string       `json:"name,omitempty"`
	AvatarURL string       `json:"avatar_url,omitempty"`
	CreatedBy uuid.UUID    `json:"created_by"`
	CreatedAt time.Time    `json:"created_at"`
	Members   []ChatMember `json:"members,omitempty"`
}

type ChatListItem struct {
	ID              uuid.UUID  `json:"id"`
	Type            string     `json:"type"`
	Name            string     `json:"name"`
	AvatarURL       string     `json:"avatar_url,omitempty"`
	PartnerID       *uuid.UUID `json:"partner_id,omitempty"`
	LastMessage     string     `json:"last_message,omitempty"`
	LastMessageTime *time.Time `json:"last_message_time,omitempty"`
	LastSenderID    *uuid.UUID `json:"last_sender_id,omitempty"`
	LastStatus      string     `json:"last_status,omitempty"`
	UnreadCount     int        `json:"unread_count"`
	CreatedAt       time.Time  `json:"created_at"`
}

type ChatMember struct {
	ChatID     uuid.UUID `json:"chat_id"`
	UserID     uuid.UUID `json:"user_id"`
	Role       string    `json:"role"`
	JoinedAt   time.Time `json:"joined_at"`
	LastReadAt time.Time `json:"last_read_at,omitempty"`
}

type CreateMessageRequest struct {
	Content string `json:"content"`
}

type CreatePrivateChatRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

type CreateGroupChatRequest struct {
	Name    string      `json:"name"`
	UserIDs []uuid.UUID `json:"user_ids"`
}

type AddMemberRequest struct {
	UserID uuid.UUID `json:"user_id"`
}
