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
	GroupName    string    `json:"group_name"`
	CreatedAt    time.Time `json:"created_at"`
}

// AcademicGroups is the single source of truth for the values accepted at
// registration. The database keeps the same five rows and enforces the FK.
var AcademicGroups = []string{"Т-491", "Т-492", "Т-493", "Т-494", "Т-495"}

func IsAcademicGroup(name string) bool {
	for _, group := range AcademicGroups {
		if group == name {
			return true
		}
	}
	return false
}

type RegisterRequest struct {
	Username  string `json:"username"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Password  string `json:"password"`
	GroupName string `json:"group_name"`
}

// LoginRequest keeps the historical `email` JSON field for compatibility, but
// the value may be either an email address or a username.
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type AuthResponse struct {
	Token string `json:"token"`
	User  *User  `json:"user"`
}

// UpdateProfileRequest intentionally has no group_name: the academic group is
// chosen once at registration and cannot be changed by this endpoint.
type UpdateProfileRequest struct {
	Name      string `json:"name"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	AvatarURL string `json:"avatar_url"`
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
	Content   string     `json:"content"`
	ReplyToID *uuid.UUID `json:"reply_to_id,omitempty"`
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
