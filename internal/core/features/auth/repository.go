package auth

import (
	"context"
	"errors"

	core_errors "messenger/internal/errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"messenger/internal/core/domain"
	"messenger/internal/db"
)

// Store describes everything the auth service needs from PostgreSQL. Keeping
// it as an interface allows the service to be unit-tested without a database.
type Store interface {
	CreateUser(ctx context.Context, user *domain.User, groupID uuid.UUID) error
	FindByIdentifier(ctx context.Context, identifier string) (*domain.User, error)
	FindByEmail(ctx context.Context, email string) (*domain.User, error)
	FindByUsername(ctx context.Context, username string) (*domain.User, error)
	FindGroupByName(ctx context.Context, name string) (uuid.UUID, error)
}

type Repository struct {
	pool *db.Pool
}

func NewRepository(pool *db.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateUser(ctx context.Context, user *domain.User, groupID uuid.UUID) error {
	name := user.Name
	if name == "" {
		name = user.Username
	}
	query := `
		INSERT INTO users (username, name, email, password_hash, group_id)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at
	`
	err := r.pool.QueryRow(ctx, query,
		user.Username, name, user.Email, user.PasswordHash, groupID,
	).Scan(&user.ID, &user.CreatedAt)
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return core_errors.ErrDuplicateUser
	}
	return err
}

const userColumns = `u.id, u.username, COALESCE(u.name, u.username), u.email, u.password_hash,
	COALESCE(u.avatar_url, ''), COALESCE(ag.name, ''), u.created_at`

const userJoins = ` LEFT JOIN academic_groups ag ON ag.id = u.group_id `

func scanUser(row interface{ Scan(...interface{}) error }) (*domain.User, error) {
	user := &domain.User{}
	err := row.Scan(
		&user.ID, &user.Username, &user.Name, &user.Email,
		&user.PasswordHash, &user.AvatarURL, &user.GroupName, &user.CreatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core_errors.ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// FindByIdentifier accepts either an email address or a username. Both sides
// are normalised in SQL so that storage casing never breaks the lookup.
func (r *Repository) FindByIdentifier(ctx context.Context, identifier string) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users u ` + userJoins + `
		WHERE LOWER(u.email) = LOWER($1) OR LOWER(u.username) = LOWER($1)
		ORDER BY CASE WHEN LOWER(u.email) = LOWER($1) THEN 0 ELSE 1 END
		LIMIT 1`
	return scanUser(r.pool.QueryRow(ctx, query, identifier))
}

func (r *Repository) FindByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users u ` + userJoins + ` WHERE LOWER(u.email) = LOWER($1) LIMIT 1`
	return scanUser(r.pool.QueryRow(ctx, query, email))
}

func (r *Repository) FindByUsername(ctx context.Context, username string) (*domain.User, error) {
	query := `SELECT ` + userColumns + ` FROM users u ` + userJoins + ` WHERE LOWER(u.username) = LOWER($1) LIMIT 1`
	return scanUser(r.pool.QueryRow(ctx, query, username))
}

func (r *Repository) FindGroupByName(ctx context.Context, name string) (uuid.UUID, error) {
	var id uuid.UUID
	err := r.pool.QueryRow(ctx, `SELECT id FROM academic_groups WHERE name = $1`, name).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, core_errors.ErrGroupNotFound
		}
		return uuid.Nil, err
	}
	return id, nil
}
