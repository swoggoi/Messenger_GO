package auth

import (
	"context"
	"errors"
	"messenger/internal/core/domain"
	"messenger/internal/db"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateUser(ctx context.Context, user *domain.User) error {
	name := user.Name
	if name == "" {
		name = user.Username
	}
	query := `INSERT INTO users (username, name, email, password_hash) VALUES ($1, $2, $3, $4) RETURNING id, created_at`
	err := r.pool.QueryRow(ctx, query, user.Username, name, user.Email, user.PasswordHash).Scan(&user.ID, &user.CreatedAt)

	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return errors.New("user already exists")
		}
		return err
	}
	return nil
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT id, username, COALESCE(name, username), email, password_hash, COALESCE(avatar_url, ''), created_at FROM users WHERE email = $1`
	user := &domain.User{}
	err := r.pool.QueryRow(ctx, query, email).Scan(&user.ID, &user.Username, &user.Name, &user.Email, &user.PasswordHash, &user.AvatarURL, &user.CreatedAt)

	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return user, nil
}
