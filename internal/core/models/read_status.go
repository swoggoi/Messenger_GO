package domain

import "time"

type ReadStatus struct {
	MessageID string    `json:"message_id"`
	UserID    string    `json:"user_id"`
	ReadAt    time.Time `json:"read_at"`
}

func NewReadStatus(messageID, userID string) *ReadStatus {
	return &ReadStatus{
		MessageID: messageID,
		UserID:    userID,
		ReadAt:    time.Now(),
	}
}
