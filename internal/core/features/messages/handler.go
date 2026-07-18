package messages

import (
	"encoding/json"
	"net/http"
	"strconv"

	"messenger/internal/core/domain"
	middleware "messenger/internal/core/transport/http/middleware"

	"github.com/google/uuid"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) CreateMessage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	chatID := r.PathValue("chatID")
	if chatID == "" {
		http.Error(w, `{"error":"invalid chat id"}`, http.StatusBadRequest)
		return
	}

	var req domain.CreateMessageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	msg, err := h.service.CreateMessage(r.Context(), chatID, userID.String(), req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(msg)
}

func (h *Handler) GetMessages(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	chatID := r.PathValue("chatID")
	if chatID == "" {
		http.Error(w, `{"error":"invalid chat id"}`, http.StatusBadRequest)
		return
	}

	limit := 50
	offset := 0

	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	if o := r.URL.Query().Get("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	h.service.DeliverPendingMessages(r.Context(), chatID, userID.String())

	messages, err := h.service.GetMessages(r.Context(), chatID, limit, offset)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	if messages == nil {
		messages = []domain.Message{}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(messages)
}

func (h *Handler) MarkAsRead(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	chatID := r.PathValue("chatID")

	if err := h.service.MarkAsRead(r.Context(), chatID, userID.String()); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "marked as read"})
}

func (h *Handler) MarkAsUnread(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	chatID := r.PathValue("chatID")

	if err := h.service.MarkAsUnread(r.Context(), chatID, userID.String()); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "marked as unread"})
}
