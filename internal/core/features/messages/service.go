package messages

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"
)

const maxMessageLength = 10000
const maxHistoryLimit = 100

type BroadcastFunc func(userIDs []uuid.UUID, event domain.WSEvent)

type Service struct {
	repo      Store
	producer  *kafka.Writer
	topic     string
	broadcast BroadcastFunc
}

func NewService(repo Store, producer *kafka.Writer, topic string) *Service {
	return &Service{repo: repo, producer: producer, topic: topic}
}

func (s *Service) SetBroadcast(fn BroadcastFunc) {
	s.broadcast = fn
}

func (s *Service) requireMember(ctx context.Context, chatID, userID uuid.UUID) error {
	isMember, err := s.repo.IsChatMember(ctx, chatID, userID)
	if err != nil {
		return err
	}
	if !isMember {
		return core_errors.ErrNotChatMember
	}
	return nil
}

func (s *Service) CreateMessage(ctx context.Context, chatID, userID uuid.UUID, req domain.CreateMessageRequest) (*domain.Message, error) {
	content := strings.TrimSpace(req.Content)
	if content == "" || len(content) > maxMessageLength {
		return nil, core_errors.ErrValidation
	}
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return nil, err
	}

	if req.ReplyToID != nil {
		target, err := s.repo.GetMessageByID(ctx, req.ReplyToID.String())
		if err != nil || target.ChatID != chatID {
			return nil, core_errors.ErrValidation
		}
	}

	msg := &domain.Message{
		ID:        uuid.New(),
		ChatID:    chatID,
		SenderID:  userID,
		Content:   content,
		Status:    "sent",
		ReplyToID: req.ReplyToID,
		CreatedAt: time.Now(),
	}
	if err := s.repo.CreateMessage(ctx, msg); err != nil {
		return nil, err
	}

	// Kafka is optional for the working minimum; publishing must never fail a send.
	if s.producer != nil {
		go func() {
			kctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			msgBytes, err := json.Marshal(msg)
			if err != nil {
				return
			}
			_ = s.producer.WriteMessages(kctx, kafka.Message{Key: []byte(chatID.String()), Value: msgBytes})
		}()
	}

	if s.broadcast != nil {
		if ids, err := s.repo.GetChatMemberIDs(ctx, chatID); err == nil {
			s.broadcast(ids, domain.WSEvent{Type: "new_message", Payload: msg})
		}
	}
	return msg, nil
}

func (s *Service) GetMessages(ctx context.Context, chatID, userID uuid.UUID, limit, offset int) ([]domain.Message, error) {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return nil, err
	}
	if limit <= 0 || limit > maxHistoryLimit {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.GetMessages(ctx, chatID, limit, offset)
}

// DeleteMessage removes a message for everyone in the chat. Only its author may do this.
func (s *Service) DeleteMessage(ctx context.Context, chatID, messageID, userID uuid.UUID) error {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return err
	}
	msg, err := s.repo.GetMessageByID(ctx, messageID.String())
	if err != nil || msg.ChatID != chatID {
		return core_errors.ErrMessageNotFound
	}
	if msg.SenderID != userID {
		return core_errors.ErrForbidden
	}
	if err := s.repo.DeleteMessage(ctx, messageID.String()); err != nil {
		return err
	}
	if s.broadcast != nil {
		if ids, err := s.repo.GetChatMemberIDs(ctx, chatID); err == nil {
			s.broadcast(ids, domain.WSEvent{Type: "message_deleted", Payload: map[string]interface{}{
				"message_id": messageID, "chat_id": chatID,
			}})
		}
	}
	return nil
}

// TogglePin pins or unpins a message for everyone in the chat.
func (s *Service) TogglePin(ctx context.Context, chatID, messageID, userID uuid.UUID) (bool, error) {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return false, err
	}
	msg, err := s.repo.GetMessageByID(ctx, messageID.String())
	if err != nil || msg.ChatID != chatID {
		return false, core_errors.ErrMessageNotFound
	}
	pinned := !msg.IsPinned
	if err := s.repo.SetMessagePinned(ctx, messageID.String(), pinned); err != nil {
		return false, err
	}
	if s.broadcast != nil {
		if ids, err := s.repo.GetChatMemberIDs(ctx, chatID); err == nil {
			s.broadcast(ids, domain.WSEvent{Type: "message_pinned", Payload: map[string]interface{}{
				"message_id": messageID, "chat_id": chatID, "is_pinned": pinned,
			}})
		}
	}
	return pinned, nil
}

func (s *Service) MarkAsRead(ctx context.Context, chatID, userID uuid.UUID) error {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return err
	}
	return s.repo.MarkChatAsRead(ctx, chatID, userID)
}

func (s *Service) MarkAsUnread(ctx context.Context, chatID, userID uuid.UUID) error {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return err
	}
	return s.repo.MarkChatAsUnread(ctx, chatID, userID)
}

func (s *Service) DeliverPendingMessages(ctx context.Context, chatID, userID uuid.UUID) error {
	if err := s.requireMember(ctx, chatID, userID); err != nil {
		return err
	}
	return s.repo.DeliverPendingMessages(ctx, chatID, userID)
}
