package chats

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
)

type fakeStore struct {
	chats       map[uuid.UUID]*domain.Chat
	members     map[uuid.UUID]map[uuid.UUID]string
	private     map[string]uuid.UUID
	createCalls int
	items       []domain.ChatListItem
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		chats:   map[uuid.UUID]*domain.Chat{},
		members: map[uuid.UUID]map[uuid.UUID]string{},
		private: map[string]uuid.UUID{},
	}
}

func privateKey(a, b uuid.UUID) string {
	if a.String() > b.String() {
		a, b = b, a
	}
	return a.String() + "|" + b.String()
}

func (f *fakeStore) CreateChat(_ context.Context, chat *domain.Chat) error {
	chat.ID = uuid.New()
	chat.CreatedAt = time.Now()
	f.chats[chat.ID] = chat
	f.members[chat.ID] = map[uuid.UUID]string{}
	return nil
}

func (f *fakeStore) CreatePrivateChat(ctx context.Context, userID, otherID uuid.UUID, chat *domain.Chat) error {
	f.createCalls++
	if existingID, ok := f.private[privateKey(userID, otherID)]; ok {
		*chat = *f.chats[existingID]
		return nil
	}
	if err := f.CreateChat(ctx, chat); err != nil {
		return err
	}
	f.private[privateKey(userID, otherID)] = chat.ID
	return f.AddMember(ctx, chat.ID, userID, "owner")
}

func (f *fakeStore) AddMember(_ context.Context, chatID, userID uuid.UUID, role string) error {
	if f.members[chatID] == nil {
		f.members[chatID] = map[uuid.UUID]string{}
	}
	f.members[chatID][userID] = role
	return nil
}

func (f *fakeStore) FindPrivateChat(_ context.Context, userID, otherID uuid.UUID) (*domain.Chat, error) {
	if id, ok := f.private[privateKey(userID, otherID)]; ok {
		return f.chats[id], nil
	}
	return nil, nil
}

func (f *fakeStore) FindSavedChat(_ context.Context, userID uuid.UUID) (*domain.Chat, error) {
	for _, chat := range f.chats {
		if chat.Type == "saved" && f.members[chat.ID][userID] != "" {
			return chat, nil
		}
	}
	return nil, nil
}

func (f *fakeStore) GetChatByID(_ context.Context, chatID uuid.UUID) (*domain.Chat, error) {
	if chat, ok := f.chats[chatID]; ok {
		return chat, nil
	}
	return nil, core_errors.ErrChatNotFound
}

func (f *fakeStore) GetMyChats(_ context.Context, _ uuid.UUID) ([]domain.ChatListItem, error) {
	return f.items, nil
}

func (f *fakeStore) GetChatsByUser(_ context.Context, _ uuid.UUID) ([]domain.Chat, error) {
	return nil, nil
}

func (f *fakeStore) GetChatMembers(_ context.Context, _ uuid.UUID) ([]domain.ChatMember, error) {
	return nil, nil
}

func (f *fakeStore) IsMember(_ context.Context, chatID, userID uuid.UUID) (bool, error) {
	return f.members[chatID][userID] != "", nil
}

func (f *fakeStore) RemoveMember(_ context.Context, chatID, userID uuid.UUID) error {
	delete(f.members[chatID], userID)
	return nil
}

func (f *fakeStore) GetMemberRole(_ context.Context, chatID, userID uuid.UUID) (string, error) {
	role := f.members[chatID][userID]
	if role == "" {
		return "", core_errors.ErrForbidden
	}
	return role, nil
}

func TestCreatePrivateChatReusesExistingChat(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	a, b := uuid.New(), uuid.New()
	first, err := service.CreatePrivateChat(context.Background(), a, b)
	if err != nil {
		t.Fatal(err)
	}
	second, err := service.CreatePrivateChat(context.Background(), b, a)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID != second.ID || store.createCalls != 1 {
		t.Fatalf("private chat duplicated: %s vs %s, creates=%d", first.ID, second.ID, store.createCalls)
	}
	if _, err := service.CreatePrivateChat(context.Background(), a, a); !errors.Is(err, core_errors.ErrValidation) {
		t.Fatalf("self-chat: got %v, want validation error", err)
	}
}

func TestChatAccessAndMemberRoles(t *testing.T) {
	store := newFakeStore()
	service := NewService(store)
	owner, member, outsider := uuid.New(), uuid.New(), uuid.New()
	chat, err := service.CreateGroupChat(context.Background(), owner, domain.CreateGroupChatRequest{
		Name: "  Учебная группа  ", UserIDs: []uuid.UUID{member},
	})
	if err != nil {
		t.Fatal(err)
	}
	if chat.Name != "Учебная группа" {
		t.Errorf("name = %q", chat.Name)
	}
	if _, err := service.GetChat(context.Background(), chat.ID, outsider); !errors.Is(err, core_errors.ErrForbidden) {
		t.Errorf("outsider read: got %v", err)
	}
	if err := service.AddMember(context.Background(), chat.ID, member, outsider); !errors.Is(err, core_errors.ErrForbidden) {
		t.Errorf("member adding: got %v", err)
	}
	if err := service.AddMember(context.Background(), chat.ID, owner, outsider); err != nil {
		t.Errorf("owner adding: %v", err)
	}
	if err := service.RemoveMember(context.Background(), chat.ID, owner, owner); !errors.Is(err, core_errors.ErrValidation) {
		t.Errorf("removing owner: got %v", err)
	}
}

func TestChatListItemJSONContract(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	partner := uuid.New()
	items := []domain.ChatListItem{{
		ID: uuid.New(), Type: "private", Name: "Иван", PartnerID: &partner,
		LastMessage: "privet", LastMessageTime: &now, LastSenderID: &partner,
		UnreadCount: 2, CreatedAt: now,
	}}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	var decoded []map[string]interface{}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "type", "name", "partner_id", "last_message", "last_message_time", "last_sender_id", "unread_count", "created_at"} {
		if _, ok := decoded[0][field]; !ok {
			t.Errorf("missing JSON field %q in %s", field, data)
		}
	}
	if _, ok := decoded[0]["members"]; ok {
		t.Error("chat list must not expose members from the old Chat DTO")
	}
}
