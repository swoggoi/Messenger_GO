package messages

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"messenger/internal/core/domain"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

type Service struct {
	repo     *Repository
	producer *kafka.Writer
	topic    string
}

func NewService(repo *Repository, producer *kafka.Writer, topic string) *Service {
	return &Service{repo: repo, producer: producer, topic: topic}
}

func (s *Service) CreateMessage(ctx context.Context, chatID, userID string, req domain.CreateMessageRequest) (*domain.Message, error) {
	if req.Content == "" {
		return nil, errors.New("message content cannot be empty")
	}

	chatUUID, err := uuid.Parse(chatID)
	if err != nil {
		return nil, errors.New("invalid chat id")
	}

	userUUID, err := uuid.Parse(userID)
	if err != nil {
		return nil, errors.New("invalid user id")
	}

	isMember, err := s.repo.IsChatMember(ctx, chatID, userID)
	if err != nil || !isMember {
		return nil, errors.New("you are not a member of this chat")
	}

	msg := &domain.Message{
		ID:        uuid.New(),
		ChatID:    chatUUID,
		SenderID:  userUUID,
		Content:   req.Content,
		Status:    "sent",
		CreatedAt: time.Now(),
	}

	
	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, errors.New("failed to save message: " + err.Error())
	}

	
	if s.producer != nil {
		msgBytes, _ := json.Marshal(msg)
		s.producer.WriteMessages(ctx, kafka.Message{
			Key:   []byte(chatID),
			Value: msgBytes,
		})
	}

	return msg, nil
}

func (s *Service) GetMessages(ctx context.Context, chatID string, limit, offset int) ([]domain.Message, error) {
	if limit <= 0 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.GetMessages(ctx, chatID, limit, offset)
}

func (s *Service) MarkAsRead(ctx context.Context, chatID, userID string) error {
	return s.repo.MarkChatAsRead(ctx, chatID, userID)
}

func (s *Service) MarkAsUnread(ctx context.Context, chatID, userID string) error {
	return s.repo.MarkChatAsUnread(ctx, chatID, userID)
}

func (s *Service) DeliverPendingMessages(ctx context.Context, chatID, userID string) error {
	return s.repo.DeliverPendingMessages(ctx, chatID, userID)
}
