package auth

import (
	"context"
	"errors"

	"golang.org/x/crypto/bcrypt"

	"messenger/internal/core/domain"
)

type Service struct {
	repo *Repository
}

func NewService(repo *Repository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Register(ctx context.Context, req domain.RegisterRequest) (*domain.User, error) {
	if req.Username == "" || req.Email == "" || req.Password == "" {
		return nil, errors.New("all fields are required")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &domain.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
	}

	if err := s.repo.CreateUser(ctx, user); err != nil {
		return nil, err
	}

	user.PasswordHash = ""
	return user, nil
}

func (s *Service) Login(ctx context.Context, email, password string) (*domain.User, error) {
	return nil, errors.New("not implemented")
}
