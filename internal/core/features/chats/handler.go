package chats

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

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

func (h *Handler) CreatePrivateChat(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	var req domain.CreatePrivateChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	chat, err := h.service.CreatePrivateChat(r.Context(), userID, req.UserID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) CreateGroupChat(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	var req domain.CreateGroupChatRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	chat, err := h.service.CreateGroupChat(r.Context(), userID, req)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) EnsureSavedChat(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	chat, err := h.service.EnsureSavedChat(r.Context(), userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) GetMyChats(w http.ResponseWriter, r *http.Request) {
	fmt.Println("=== GetMyChats called ===")
	fmt.Println("Method:", r.Method)
	fmt.Println("URL:", r.URL.String())
	fmt.Println("Headers:", r.Header)

	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		fmt.Println("ERROR: userID is nil")
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	fmt.Println("UserID:", userID)

	chats, err := h.service.GetChats(r.Context(), userID.(uuid.UUID))
	if err != nil {
		fmt.Println("ERROR getting chats:", err)
		http.Error(w, `{"error":"failed to load chats"}`, http.StatusInternalServerError)
		return
	}

	fmt.Println("Chats loaded:", len(chats))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chats)
}

func (h *Handler) GetChat(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	chatID, err := uuid.Parse(r.PathValue("chatID"))
	if err != nil {
		http.Error(w, `{"error":"invalid chat id"}`, http.StatusBadRequest)
		return
	}

	chat, err := h.service.GetChat(r.Context(), chatID, userID)
	if err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusForbidden)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(chat)
}

func (h *Handler) AddMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	parts := strings.Split(r.URL.Path, "/")
	chatID, err := uuid.Parse(parts[4])
	if err != nil {
		http.Error(w, `{"error":"invalid chat id"}`, http.StatusBadRequest)
		return
	}

	var req domain.AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.AddMember(r.Context(), chatID, userID, req.UserID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"member added"}`))
}

func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey).(uuid.UUID)

	parts := strings.Split(r.URL.Path, "/")
	chatID, err := uuid.Parse(parts[4])
	if err != nil {
		http.Error(w, `{"error":"invalid chat id"}`, http.StatusBadRequest)
		return
	}

	memberID, err := uuid.Parse(parts[6])
	if err != nil {
		http.Error(w, `{"error":"invalid member id"}`, http.StatusBadRequest)
		return
	}

	if err := h.service.RemoveMember(r.Context(), chatID, userID, memberID); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"member removed"}`))
}
