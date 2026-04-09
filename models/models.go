package models

import "time"

type User struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Username  string    `json:"username" gorm:"uniqueIndex;size:50;not null"`
	CreatedAt time.Time `json:"created_at" gorm:"default:current_timestamp"`
}

type Conversation struct {
	ID        uint      `json:"id" gorm:"primaryKey"`
	Name      string    `json:"name" gorm:"size:100"`
	CreatedAt time.Time `json:"created_at" gorm:"default:current_timestamp"`
}

type ConversationMember struct {
	ConversationID uint `json:"-" gorm:"primaryKey"`
	UserID         uint `json:"-" gorm:"primaryKey"`
}

type Message struct {
	ID             uint      `json:"id" gorm:"primaryKey"`
	ConversationID uint      `json:"conversation_id" gorm:"index"`
	UserID         uint      `json:"user_id" gorm:"index"`
	Text           string    `json:"text" gorm:"not null;type:text"`
	CreatedAt      time.Time `json:"created_at" gorm:"default:current_timestamp;index"`
}
