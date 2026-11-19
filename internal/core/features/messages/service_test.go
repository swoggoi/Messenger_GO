package messages

import (
	"context"
	"errors"
	"testing"
	"time"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
)

type fakeStore struct {
	members  map[uuid.UUID]bool
	messages []domain.Message
	created  int
}

func (f *fakeStore) CreateMessage(_ context.Context, msg *domain.Message) error {
	f.created++
	msg.SenderName = "Иван Иванов"
	f.messages = append(f.messages, *msg)
	return nil
}

func (f *fakeStore) GetMessageByID(_ context.Context, messageID string) (*domain.Message, error) {
	id, err := uuid.Parse(messageID)
	if err != nil {
		return nil, core_errors.ErrMessageNotFound
	}
	for i := range f.messages {
		if f.messages[i].ID == id {
			msg := f.messages[i]
			return &msg, nil
		}
	}
	return nil, core_errors.ErrMessageNotFound
}

func (f *fakeStore) DeleteMessage(_ context.Context, messageID string) error {
	id, err := uuid.Parse(messageID)
	if err != nil {
		return core_errors.ErrMessageNotFound
	}
	for i := range f.messages {
		if f.messages[i].ID == id {
			now := time.Now()
			f.messages[i].DeletedAt = &now
			return nil
		}
	}
	return core_errors.ErrMessageNotFound
}

func (f *fakeStore) SetMessagePinned(_ context.Context, messageID string, pinned bool) error {
	id, err := uuid.Parse(messageID)
	if err != nil {
		return core_errors.ErrMessageNotFound
	}
	for i := range f.messages {
		if f.messages[i].ID == id {
			f.messages[i].IsPinned = pinned
			return nil
		}
	}
	return core_errors.ErrMessageNotFound
}

func (f *fakeStore) GetMessages(_ context.Context, _ uuid.UUID, limit, offset int) ([]domain.Message, error) {
	if offset >= len(f.messages) {
		return []domain.Message{}, nil
	}
	end := offset + limit
	if end > len(f.messages) {
		end = len(f.messages)
	}
	return f.messages[offset:end], nil
}

func (f *fakeStore) IsChatMember(_ context.Context, _, userID uuid.UUID) (bool, error) {
	return f.members[userID], nil
}

func (f *fakeStore) GetChatMemberIDs(_ context.Context, _ uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	for id := range f.members {
		ids = append(ids, id)
	}
	return ids, nil
}

func (f *fakeStore) MarkChatAsRead(context.Context, uuid.UUID, uuid.UUID) error   { return nil }
func (f *fakeStore) MarkChatAsUnread(context.Context, uuid.UUID, uuid.UUID) error { return nil }
func (f *fakeStore) DeliverPendingMessages(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func TestNonMemberCannotReadSendOrMark(t *testing.T) {
	store := &fakeStore{members: map[uuid.UUID]bool{uuid.New(): true}}
	service := NewService(store, nil, "topic")
	chatID, outsider := uuid.New(), uuid.New()

	if _, err := service.GetMessages(context.Background(), chatID, outsider, 50, 0); !errors.Is(err, core_errors.ErrNotChatMember) {
		t.Errorf("read: got %v", err)
	}
	if _, err := service.CreateMessage(context.Background(), chatID, outsider, domain.CreateMessageRequest{Content: "hi"}); !errors.Is(err, core_errors.ErrNotChatMember) {
		t.Errorf("send: got %v", err)
	}
	if err := service.MarkAsRead(context.Background(), chatID, outsider); !errors.Is(err, core_errors.ErrNotChatMember) {
		t.Errorf("read mark: got %v", err)
	}
	if store.created != 0 {
		t.Error("message was persisted for a non-member")
	}
}

func TestHistoryIncludesSenderNameAndClampsLimit(t *testing.T) {
	sender := uuid.New()
	store := &fakeStore{members: map[uuid.UUID]bool{sender: true}}
	service := NewService(store, nil, "topic")
	chatID := uuid.New()
	msg, err := service.CreateMessage(context.Background(), chatID, sender, domain.CreateMessageRequest{Content: "  privet  "})
	if err != nil {
		t.Fatal(err)
	}
	if msg.SenderName != "Иван Иванов" {
		t.Errorf("sender_name = %q", msg.SenderName)
	}
	if msg.Content != "privet" {
		t.Errorf("content = %q", msg.Content)
	}
	history, err := service.GetMessages(context.Background(), chatID, sender, 1000, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0].ID != msg.ID || history[0].SenderName == "" {
		t.Fatalf("unexpected history: %+v", history)
	}
	if history[0].CreatedAt.IsZero() {
		t.Error("created_at was not populated")
	}
	if _, err := service.CreateMessage(context.Background(), chatID, sender, domain.CreateMessageRequest{Content: "   "}); !errors.Is(err, core_errors.ErrValidation) {
		t.Errorf("empty content: got %v", err)
	}
}

func TestBroadcastPayloadIsWSEvent(t *testing.T) {
	sender := uuid.New()
	store := &fakeStore{members: map[uuid.UUID]bool{sender: true}}
	service := NewService(store, nil, "topic")
	var got domain.WSEvent
	service.SetBroadcast(func(ids []uuid.UUID, event domain.WSEvent) { got = event })
	msg, err := service.CreateMessage(context.Background(), uuid.New(), sender, domain.CreateMessageRequest{Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if got.Type != "new_message" {
		t.Fatalf("event type = %q", got.Type)
	}
	payload, ok := got.Payload.(*domain.Message)
	if !ok || payload.ID != msg.ID || payload.SenderName == "" {
		t.Fatalf("unexpected payload: %#v", got.Payload)
	}
}

func TestDeleteMessageOnlyAuthorAndBroadcast(t *testing.T) {
	author, other := uuid.New(), uuid.New()
	store := &fakeStore{members: map[uuid.UUID]bool{author: true, other: true}}
	service := NewService(store, nil, "topic")
	var event domain.WSEvent
	service.SetBroadcast(func(_ []uuid.UUID, e domain.WSEvent) { event = e })
	chatID := uuid.New()
	msg, err := service.CreateMessage(context.Background(), chatID, author, domain.CreateMessageRequest{Content: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if err := service.DeleteMessage(context.Background(), chatID, msg.ID, other); !errors.Is(err, core_errors.ErrForbidden) {
		t.Fatalf("other user delete: got %v", err)
	}
	if err := service.DeleteMessage(context.Background(), chatID, msg.ID, author); err != nil {
		t.Fatal(err)
	}
	if store.messages[0].DeletedAt == nil {
		t.Error("message was not soft-deleted")
	}
	if event.Type != "message_deleted" {
		t.Errorf("event type = %q", event.Type)
	}
}

func TestTogglePinAndReplyValidation(t *testing.T) {
	sender := uuid.New()
	store := &fakeStore{members: map[uuid.UUID]bool{sender: true}}
	service := NewService(store, nil, "topic")
	chatID := uuid.New()
	msg, err := service.CreateMessage(context.Background(), chatID, sender, domain.CreateMessageRequest{Content: "original"})
	if err != nil {
		t.Fatal(err)
	}
	pinned, err := service.TogglePin(context.Background(), chatID, msg.ID, sender)
	if err != nil || !pinned || !store.messages[0].IsPinned {
		t.Fatalf("pin: pinned=%v err=%v", pinned, err)
	}
	pinned, err = service.TogglePin(context.Background(), chatID, msg.ID, sender)
	if err != nil || pinned {
		t.Fatalf("unpin: pinned=%v err=%v", pinned, err)
	}
	if _, err := service.CreateMessage(context.Background(), chatID, sender, domain.CreateMessageRequest{
		Content: "reply", ReplyToID: &[]uuid.UUID{uuid.New()}[0],
	}); !errors.Is(err, core_errors.ErrValidation) {
		t.Errorf("reply from another chat: got %v", err)
	}
	reply, err := service.CreateMessage(context.Background(), chatID, sender, domain.CreateMessageRequest{
		Content: "reply", ReplyToID: &msg.ID,
	})
	if err != nil || reply.ReplyToID == nil || *reply.ReplyToID != msg.ID {
		t.Fatalf("valid reply: msg=%+v err=%v", reply, err)
	}
}
