package auth

import (
	"encoding/json"
	"errors"
	"net/http"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"
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

func writeServiceError(w http.ResponseWriter, err error, invalidCredentialsStatus int) {
	switch {
	case errors.Is(err, core_errors.ErrValidation):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, core_errors.ErrDuplicateUser):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, core_errors.ErrUserNotFound), errors.Is(err, core_errors.ErrInvalidCredentials):
		writeError(w, invalidCredentialsStatus, "invalid credentials")
	case errors.Is(err, core_errors.ErrGroupNotFound):
		writeError(w, http.StatusBadRequest, "academic group not found")
	default:
		writeError(w, http.StatusInternalServerError, "internal server error")
	}
}

func (h *Handler) Register(w http.ResponseWriter, r *http.Request) {
	var req domain.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	user, err := h.service.Register(r.Context(), req)
	if err != nil {
		writeServiceError(w, err, http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req domain.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid json")
		return
	}

	resp, err := h.service.Login(r.Context(), req)
	if err != nil {
		writeServiceError(w, err, http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
