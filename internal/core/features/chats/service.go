package chats

import (
	"context"
	"errors"
	"strings"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
)

type Service struct {
	repo Store
}

func NewService(repo Store) *Service {
	return &Service{repo: repo}
}

func (s *Service) EnsureSavedChat(ctx context.Context, userID uuid.UUID) (*domain.Chat, error) {
	existing, err := s.repo.FindSavedChat(ctx, userID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	chat := &domain.Chat{Type: "saved", Name: "Избранное", CreatedBy: userID}
	if err := s.repo.CreateChat(ctx, chat); err != nil {
		// Другой параллельный запрос мог создать чат первым.
		if existing, findErr := s.repo.FindSavedChat(ctx, userID); findErr == nil && existing != nil {
			return existing, nil
		}
		return nil, err
	}
	if err := s.repo.AddMember(ctx, chat.ID, userID, "owner"); err != nil {
		return nil, err
	}
	return chat, nil
}

func (s *Service) CreatePrivateChat(ctx context.Context, userID, otherUserID uuid.UUID) (*domain.Chat, error) {
	if userID == otherUserID {
		return nil, core_errors.ErrValidation
	}
	if userID == uuid.Nil || otherUserID == uuid.Nil {
		return nil, core_errors.ErrValidation
	}

	existing, err := s.repo.FindPrivateChat(ctx, userID, otherUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	chat := &domain.Chat{Type: "private", CreatedBy: userID}
	if err := s.repo.CreatePrivateChat(ctx, userID, otherUserID, chat); err != nil {
		return nil, err
	}
	return chat, nil
}

func (s *Service) GetChats(ctx context.Context, userID uuid.UUID) ([]domain.Chat, error) {
	chats, err := s.repo.GetChatsByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	for i := range chats {
		members, err := s.repo.GetChatMembers(ctx, chats[i].ID)
		if err != nil {
			return nil, err
		}
		chats[i].Members = members
	}
	return chats, nil
}

func (s *Service) CreateGroupChat(ctx context.Context, userID uuid.UUID, req domain.CreateGroupChatRequest) (*domain.Chat, error) {
	name := strings.Join(strings.Fields(req.Name), " ")
	if name == "" || len(name) > 100 {
		return nil, core_errors.ErrValidation
	}
	if len(req.UserIDs) == 0 {
		return nil, core_errors.ErrValidation
	}
	seen := map[uuid.UUID]bool{userID: true}
	members := make([]uuid.UUID, 0, len(req.UserIDs))
	for _, id := range req.UserIDs {
		if id == uuid.Nil || seen[id] {
			continue
		}
		seen[id] = true
		members = append(members, id)
	}
	if len(members) == 0 {
		return nil, core_errors.ErrValidation
	}

	chat := &domain.Chat{Type: "group", Name: name, CreatedBy: userID}
	if err := s.repo.CreateChat(ctx, chat); err != nil {
		return nil, err
	}
	if err := s.repo.AddMember(ctx, chat.ID, userID, "owner"); err != nil {
		return nil, err
	}
	for _, memberID := range members {
		if err := s.repo.AddMember(ctx, chat.ID, memberID, "member"); err != nil {
			return nil, err
		}
	}
	return chat, nil
}

// GetMyChats returns the stable ChatListItem DTO consumed by web/app.js.
func (s *Service) GetMyChats(ctx context.Context, userID uuid.UUID) ([]domain.ChatListItem, error) {
	if _, err := s.EnsureSavedChat(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.GetMyChats(ctx, userID)
}

func (s *Service) GetChat(ctx context.Context, chatID, userID uuid.UUID) (*domain.Chat, error) {
	isMember, err := s.repo.IsMember(ctx, chatID, userID)
	if err != nil {
		return nil, err
	}
	if !isMember {
		return nil, core_errors.ErrForbidden
	}
	chat, err := s.repo.GetChatByID(ctx, chatID)
	if err != nil {
		return nil, err
	}
	members, err := s.repo.GetChatMembers(ctx, chatID)
	if err != nil {
		return nil, err
	}
	chat.Members = members
	return chat, nil
}

func (s *Service) AddMember(ctx context.Context, chatID, userID, newMemberID uuid.UUID) error {
	if newMemberID == uuid.Nil {
		return core_errors.ErrValidation
	}
	role, err := s.repo.GetMemberRole(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, core_errors.ErrForbidden) {
			return core_errors.ErrForbidden
		}
		return err
	}
	if role != "owner" && role != "admin" {
		return core_errors.ErrForbidden
	}
	chat, err := s.repo.GetChatByID(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.Type != "group" {
		return core_errors.ErrValidation
	}
	return s.repo.AddMember(ctx, chatID, newMemberID, "member")
}

func (s *Service) RemoveMember(ctx context.Context, chatID, userID, memberID uuid.UUID) error {
	if memberID == uuid.Nil {
		return core_errors.ErrValidation
	}
	role, err := s.repo.GetMemberRole(ctx, chatID, userID)
	if err != nil {
		if errors.Is(err, core_errors.ErrForbidden) {
			return core_errors.ErrForbidden
		}
		return err
	}
	if role != "owner" && role != "admin" {
		return core_errors.ErrForbidden
	}
	chat, err := s.repo.GetChatByID(ctx, chatID)
	if err != nil {
		return err
	}
	if chat.Type != "group" {
		return core_errors.ErrValidation
	}
	if memberID == chat.CreatedBy {
		return core_errors.ErrValidation
	}
	return s.repo.RemoveMember(ctx, chatID, memberID)
}
