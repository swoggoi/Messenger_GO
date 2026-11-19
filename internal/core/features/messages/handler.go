package messages

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"messenger/internal/core/domain"
	middleware "messenger/internal/core/transport/http/middleware"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core_errors.ErrValidation):
		writeError(w, http.StatusBadRequest, "invalid message")
	case errors.Is(err, core_errors.ErrNotChatMember), errors.Is(err, core_errors.ErrForbidden):
		writeError(w, http.StatusForbidden, "access denied")
	case errors.Is(err, core_errors.ErrChatNotFound), errors.Is(err, core_errors.ErrMessageNotFound):
		writeError(w, http.StatusNotFound, "message not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func chatAndUser(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	userID, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || userID == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, uuid.Nil, false
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return uuid.Nil, uuid.Nil, false
	}
	return chatID, userID, true
}

func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := chatAndUser(w, r)
	if !ok {
		return
	}
	var req domain.CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	msg, err := h.service.CreateMessage(r.Context(), chatID, userID, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(msg)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := chatAndUser(w, r)
	if !ok {
		return
	}

	limit, offset := 50, 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			limit = n
		}
	}
	if raw := r.URL.Query().Get("offset"); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n >= 0 {
			offset = n
		}
	}

	if err := h.service.DeliverPendingMessages(r.Context(), chatID, userID); err != nil {
		writeServiceError(w, err)
		return
	}
	messages, err := h.service.GetMessages(r.Context(), chatID, userID, limit, offset)
	if err != nil {
		writeServiceError(w, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

func chatUserAndMessage(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, uuid.UUID, bool) {
	chatID, userID, ok := chatAndUser(w, r)
	if !ok {
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	messageID, err := uuid.Parse(r.PathValue("messageID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid message id")
		return uuid.Nil, uuid.Nil, uuid.Nil, false
	}
	return chatID, userID, messageID, true
}

func (h *Handler) DeleteMessage(w http.ResponseWriter, r *http.Request) {
	chatID, userID, messageID, ok := chatUserAndMessage(w, r)
	if !ok {
		return
	}
	if err := h.service.DeleteMessage(r.Context(), chatID, messageID, userID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func (h *Handler) TogglePin(w http.ResponseWriter, r *http.Request) {
	chatID, userID, messageID, ok := chatUserAndMessage(w, r)
	if !ok {
		return
	}
	pinned, err := h.service.TogglePin(r.Context(), chatID, messageID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"is_pinned": pinned})
}

func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := chatAndUser(w, r)
	if !ok {
		return
	}
	if err := h.service.MarkAsRead(r.Context(), chatID, userID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "marked as read"})
}

func (h *Handler) MarkAsUnread(w http.ResponseWriter, r *http.Request) {
	chatID, userID, ok := chatAndUser(w, r)
	if !ok {
		return
	}
	if err := h.service.MarkAsUnread(r.Context(), chatID, userID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "marked as unread"})
}
