package users

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"messenger/internal/core/domain"
	middleware "messenger/internal/core/transport/http/middleware"
	"messenger/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Handler struct {
	pool *db.Pool
}

func NewHandler(pool *db.Pool) *Handler {
	return &Handler{pool: pool}
}

func (h *Handler) getUserByID(ctx context.Context, userID interface{}) (*domain.User, error) {
	var user domain.User
	query := `SELECT id, username, COALESCE(name, username), email, COALESCE(avatar_url, ''), created_at FROM users WHERE id = $1`
	err := h.pool.QueryRow(ctx, query, userID).Scan(
		&user.ID, &user.Username, &user.Name, &user.Email, &user.AvatarURL, &user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	user, err := h.getUserByID(context.Background(), userID)
	if err != nil {
		if err.Error() == "user not found" {
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
			return
		}
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func (h *Handler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value(middleware.UserIDKey)
	if userID == nil {
		http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
		return
	}

	var req struct {
		Name      string `json:"name"`
		Username  string `json:"username"`
		Email     string `json:"email"`
		AvatarURL string `json:"avatar_url"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid json"}`, http.StatusBadRequest)
		return
	}

	updates := []string{}
	params := []interface{}{}
	paramCount := 1

	if req.Name != "" {
		updates = append(updates, "name = $"+fmt.Sprint(paramCount))
		params = append(params, req.Name)
		paramCount++
	}
	if req.Username != "" {
		updates = append(updates, "username = $"+fmt.Sprint(paramCount))
		params = append(params, req.Username)
		paramCount++
	}
	if req.Email != "" {
		updates = append(updates, "email = $"+fmt.Sprint(paramCount))
		params = append(params, req.Email)
		paramCount++
	}
	if req.AvatarURL != "" {
		updates = append(updates, "avatar_url = $"+fmt.Sprint(paramCount))
		params = append(params, req.AvatarURL)
		paramCount++
	}

	if len(updates) == 0 {
		http.Error(w, `{"error":"no fields to update"}`, http.StatusBadRequest)
		return
	}

	updates = append(updates, "updated_at = CURRENT_TIMESTAMP")
	query := "UPDATE users SET " + strings.Join(updates, ", ") + " WHERE id = $" + fmt.Sprint(paramCount)
	params = append(params, userID)

	_, err := h.pool.Exec(context.Background(), query, params...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			http.Error(w, `{"error":"username или email уже заняты"}`, http.StatusConflict)
			return
		}
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	user, err := h.getUserByID(context.Background(), userID)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "updated",
		"user":   user,
	})
}

func (h *Handler) SearchUsers(w http.ResponseWriter, r *http.Request) {
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query == "" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]domain.User{})
		return
	}

	searchQuery := "%" + strings.ToLower(query) + "%"
	sql := `
		SELECT id, username, COALESCE(name, username), email, COALESCE(avatar_url, '')
		FROM users
		WHERE LOWER(username) LIKE $1
		   OR LOWER(COALESCE(name, username)) LIKE $1
		   OR LOWER(email) LIKE $1
		ORDER BY username
		LIMIT 20
	`

	rows, err := h.pool.Query(context.Background(), sql, searchQuery)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		var user domain.User
		if err := rows.Scan(&user.ID, &user.Username, &user.Name, &user.Email, &user.AvatarURL); err != nil {
			continue
		}
		users = append(users, user)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(users)
}
