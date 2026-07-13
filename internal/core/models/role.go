package domain

import "time"

type Role struct {
	ID        string    `json:"id"`
	ChatID    string    `json:"chat_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color,omitempty"`
	Priority  int       `json:"priority"`
	CreatedAt time.Time `json:"created_at"`
}

func NewRole(id, chatID, name, color string, priority int) *Role {
	return &Role{
		ID:        id,
		ChatID:    chatID,
		Name:      name,
		Color:     color,
		Priority:  priority,
		CreatedAt: time.Now(),
	}
}
