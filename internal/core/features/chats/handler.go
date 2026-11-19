package chats

import (
	"encoding/json"
	"errors"
	"net/http"

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
		writeError(w, http.StatusBadRequest, "invalid request")
	case errors.Is(err, core_errors.ErrForbidden):
		writeError(w, http.StatusForbidden, "access denied")
	case errors.Is(err, core_errors.ErrChatNotFound), errors.Is(err, core_errors.ErrUserNotFound):
		writeError(w, http.StatusNotFound, "not found")
	case errors.Is(err, core_errors.ErrDuplicateUser):
		writeError(w, http.StatusConflict, "already exists")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func requestUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || id == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) CreatePrivateChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	var req domain.CreatePrivateChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	chat, err := h.service.CreatePrivateChat(r.Context(), userID, req.UserID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) CreateGroupChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	var req domain.CreateGroupChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	chat, err := h.service.CreateGroupChat(r.Context(), userID, req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) EnsureSavedChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	chat, err := h.service.EnsureSavedChat(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) GetMyChats(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	chats, err := h.service.GetMyChats(r.Context(), userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chats)
}

func (h *Handler) GetChat(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	chat, err := h.service.GetChat(r.Context(), chatID, userID)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	var req domain.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	if err := h.service.AddMember(r.Context(), chatID, userID, req.UserID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "member added"})
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID, ok := requestUserID(w, r)
	if !ok {
		return
	}
	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid chat id")
		return
	}
	memberID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid member id")
		return
	}
	if err := h.service.RemoveMember(r.Context(), chatID, userID, memberID); err != nil {
		writeServiceError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "member removed"})
}
