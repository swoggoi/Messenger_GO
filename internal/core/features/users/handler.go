package users

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	"messenger/internal/core/domain"
	middleware "messenger/internal/core/transport/http/middleware"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
	emailPattern    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

type Handler struct {
	store Store
}

func NewHandler(store Store) *Handler {
	return &Handler{store: store}
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

func currentUserID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, ok := r.Context().Value(middleware.UserIDKey).(uuid.UUID)
	if !ok || id == uuid.Nil {
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return uuid.Nil, false
	}
	return id, true
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	user, err := h.store.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, core_errors.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

// GetUser returns public fields of another user (no password hash, no group id).
func (h *Handler) GetUser(w http.ResponseWriter, r *http.Request) {
	if _, ok := currentUserID(w, r); !ok {
		return
	}
	userID, err := uuid.Parse(r.PathValue("userID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid user id")
		return
	}
	user, err := h.store.GetByID(r.Context(), userID)
	if err != nil {
		if errors.Is(err, core_errors.ErrUserNotFound) {
			writeError(w, http.StatusNotFound, "user not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}

	var req domain.UpdateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}
	req.Name = strings.Join(strings.Fields(req.Name), " ")
	req.Username = strings.ToLower(strings.TrimSpace(req.Username))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	req.AvatarURL = strings.TrimSpace(req.AvatarURL)

	switch {
	case req.Name == "" || len(req.Name) > 100:
		writeError(w, http.StatusBadRequest, "укажите имя")
		return
	case !usernamePattern.MatchString(req.Username):
		writeError(w, http.StatusBadRequest, "username должен содержать 3–32 символа (латиница, цифры, подчёркивание)")
		return
	case !emailPattern.MatchString(req.Email) || len(req.Email) > 255:
		writeError(w, http.StatusBadRequest, "введите корректный адрес электронной почты")
		return
	}

	user, err := h.store.UpdateProfile(r.Context(), userID, req)
	if err != nil {
		switch {
		case errors.Is(err, core_errors.ErrDuplicateUser), errors.Is(err, core_errors.ErrValidation):
			writeError(w, http.StatusConflict, err.Error())
		case errors.Is(err, core_errors.ErrUserNotFound):
			writeError(w, http.StatusNotFound, "user not found")
		default:
			writeError(w, http.StatusInternalServerError, "internal server error")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "updated",
		"user":   user,
	})
}

func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := currentUserID(w, r)
	if !ok {
		return
	}
	// Поиск по @username должен работать так же, как по username без символа «@».
	query := strings.TrimPrefix(strings.TrimSpace(r.URL.Query().Get("q")), "@")
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domain.User{})
		return
	}

	users, err := h.store.Search(r.Context(), userID, query)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "internal server error")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}
