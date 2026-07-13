package domain

import "time"

type Channel struct {
	ChannelID   string    `json:"id"`
	Name        string    `json:"name"`
	Username    string    `json:"username"`
	Description string    `json:"description"`
	OwnerID     string    `json:"owner_id"`
	Icon        string    `json:"icon"`
	CreatedAt   time.Time `json:"created_at"`
}

func NewChannel(channelID, name, username, description, ownerID, icon string) *Channel {
	return &Channel{
		ChannelID:   channelID,
		Name:        name,
		Username:    username,
		Description: description,
		OwnerID:     ownerID,
		Icon:        icon,
		CreatedAt:   time.Now(),
	}
}
