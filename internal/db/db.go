package db

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

type Pool struct {
	*pgxpool.Pool
}

func NewPool(ctx context.Context, dbSource string, logger *zap.Logger) (*Pool, error) {
	config, err := pgxpool.ParseConfig(dbSource)
	if err != nil {
		return nil, err
	}

	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		return nil, err
	}

	logger.Info("postgres connection pool initialized")
	return &Pool{pool}, nil
}
