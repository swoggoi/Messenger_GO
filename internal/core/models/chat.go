package domain

import "time"

type Chat struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	IsGroup     bool      `json:"is_group"`
	Icon        string    `json:"icon,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewChat(id, name, description string, isGroup bool, icon string) *Chat {
	return &Chat{
		ID:          id,
		Name:        name,
		Description: description,
		IsGroup:     isGroup,
		Icon:        icon,
		CreatedAt:   time.Now(),
	}
}
