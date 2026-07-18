package chats

import (
	"context"
	"errors"

	"messenger/internal/core/domain"

	"github.com/google/uuid"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
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

	chat := &domain.Chat{
		Type:      "saved",
		Name:      "Избранное",
		CreatedBy: userID,
	}

	if err := s.repo.CreateChat(ctx, chat); err != nil {
		return nil, err
	}

	if err := s.repo.AddMember(ctx, chat.ID, userID, "owner"); err != nil {
		return nil, err
	}

	return chat, nil
}

func (s *Service) CreatePrivateChat(ctx context.Context, userID, otherUserID uuid.UUID) (*domain.Chat, error) {
	if userID == otherUserID {
		return nil, errors.New("нельзя написать себе — используйте «Избранное»")
	}

	existing, err := s.repo.FindPrivateChat(ctx, userID, otherUserID)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return existing, nil
	}

	chat := &domain.Chat{
		Type:      "private",
		CreatedBy: userID,
	}

	if err := s.repo.CreateChat(ctx, chat); err != nil {
		return nil, err
	}

	if err := s.repo.AddMember(ctx, chat.ID, userID, "owner"); err != nil {
		return nil, err
	}

	if err := s.repo.AddMember(ctx, chat.ID, otherUserID, "member"); err != nil {
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
		chatMembers, err := s.repo.GetChatMembers(ctx, chats[i].ID)
		if err != nil {
			return nil, err
		}
		chats[i].Members = chatMembers
	}

	return chats, nil
}
func (s *Service) CreateGroupChat(ctx context.Context, userID uuid.UUID, req domain.CreateGroupChatRequest) (*domain.Chat, error) {
	if req.Name == "" {
		return nil, errors.New("group name is required")
	}

	if len(req.UserIDs) == 0 {
		return nil, errors.New("at least one member is required")
	}

	chat := &domain.Chat{
		Type:      "group",
		Name:      req.Name,
		CreatedBy: userID,
	}

	if err := s.repo.CreateChat(ctx, chat); err != nil {
		return nil, err
	}

	if err := s.repo.AddMember(ctx, chat.ID, userID, "owner"); err != nil {
		return nil, err
	}

	for _, memberID := range req.UserIDs {
		if memberID != userID {
			if err := s.repo.AddMember(ctx, chat.ID, memberID, "member"); err != nil {
				return nil, err
			}
		}
	}

	return chat, nil
}

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
		return nil, errors.New("access denied")
	}

	return s.repo.GetChatByID(ctx, chatID)
}

func (s *Service) AddMember(ctx context.Context, chatID, userID, newMemberID uuid.UUID) error {
	role, err := s.repo.GetMemberRole(ctx, chatID, userID)
	if err != nil {
		return errors.New("access denied")
	}

	if role != "owner" && role != "admin" {
		return errors.New("only owner or admin can add members")
	}

	chat, err := s.repo.GetChatByID(ctx, chatID)
	if err != nil {
		return err
	}

	if chat.Type != "group" {
		return errors.New("cannot add members to private chat")
	}

	return s.repo.AddMember(ctx, chatID, newMemberID, "member")
}

func (s *Service) RemoveMember(ctx context.Context, chatID, userID, memberID uuid.UUID) error {
	role, err := s.repo.GetMemberRole(ctx, chatID, userID)
	if err != nil {
		return errors.New("access denied")
	}

	if role != "owner" && role != "admin" {
		return errors.New("only owner or admin can remove members")
	}

	return s.repo.RemoveMember(ctx, chatID, memberID)
}
