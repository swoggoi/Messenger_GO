package users

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"messenger/internal/core/domain"
	"messenger/internal/db"
	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Store keeps the HTTP layer independent of pgx so handlers can be tested.
type Store interface {
	GetByID(ctx context.Context, userID uuid.UUID) (*domain.User, error)
	Search(ctx context.Context, excludeUserID uuid.UUID, query string) ([]domain.User, error)
	UpdateProfile(ctx context.Context, userID uuid.UUID, req domain.UpdateProfileRequest) (*domain.User, error)
}

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

const userColumns = `u.id, u.username, COALESCE(u.name, u.username), u.email,
	COALESCE(u.avatar_url, ''), COALESCE(ag.name, ''), u.created_at`

const userJoins = ` FROM users u LEFT JOIN academic_groups ag ON ag.id = u.group_id `

func scanUser(row interface{ Scan(...interface{}) error }) (*domain.User, error) {
	user := &domain.User{}
	err := row.Scan(
		&user.ID, &user.Username, &user.Name, &user.Email,
		&user.AvatarURL, &user.GroupName, &user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core_errors.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

func (r *Repository) GetByID(ctx context.Context, userID uuid.UUID) (*domain.User, error) {
	query := `SELECT ` + userColumns + userJoins + ` WHERE u.id = $1`
	return scanUser(r.pool.QueryRow(ctx, query, userID))
}

func (r *Repository) Search(ctx context.Context, excludeUserID uuid.UUID, query string) ([]domain.User, error) {
	like := "%" + strings.ToLower(query) + "%"
	sql := `SELECT ` + userColumns + userJoins + `
		WHERE u.id != $2 AND (
			LOWER(u.username) LIKE $1
			OR LOWER(COALESCE(u.name, u.username)) LIKE $1
			OR LOWER(u.email) LIKE $1
		)
		ORDER BY u.username
		LIMIT 20`
	rows, err := r.pool.Query(ctx, sql, like, excludeUserID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	users := []domain.User{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *user)
	}
	return users, rows.Err()
}

func (r *Repository) UpdateProfile(ctx context.Context, userID uuid.UUID, req domain.UpdateProfileRequest) (*domain.User, error) {
	updates := []string{}
	params := []interface{}{}
	add := func(column, value string) {
		params = append(params, value)
		updates = append(updates, fmt.Sprintf("%s = $%d", column, len(params)))
	}

	if req.Name != "" {
		add("name", req.Name)
	}
	if req.Username != "" {
		add("username", req.Username)
	}
	if req.Email != "" {
		add("email", req.Email)
	}
	if req.AvatarURL != "" {
		add("avatar_url", req.AvatarURL)
	}
	if len(updates) == 0 {
		return nil, fmt.Errorf("%w: no fields to update", core_errors.ErrValidation)
	}
	params = append(params, userID)
	query := "UPDATE users SET " + strings.Join(updates, ", ") +
		fmt.Sprintf(", updated_at = CURRENT_TIMESTAMP WHERE id = $%d", len(params))

	_, err := r.pool.Exec(ctx, query, params...)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, fmt.Errorf("%w: пользователь с таким username или почтой уже существует", core_errors.ErrDuplicateUser)
		}
		return nil, err
	}
	return r.GetByID(ctx, userID)
}
