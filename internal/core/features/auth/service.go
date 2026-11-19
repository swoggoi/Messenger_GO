package auth

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	usernamePattern = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
	emailPattern    = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]+$`)
)

type Service struct {
	repo      Store
	jwtSecret string
}

func NewService(repo Store, jwtSecret string) *Service {
	return &Service{repo: repo, jwtSecret: jwtSecret}
}

func normalizeUsername(value string) string {
	// Username может прийти с ведущим «@» (из профиля, поиска или формы входа).
	return strings.ToLower(strings.TrimPrefix(strings.TrimSpace(value), "@"))
}

func normalizeEmail(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func validPassword(password string) bool {
	if len(password) < 8 {
		return false
	}
	return strings.IndexFunc(password, func(r rune) bool { return r >= '0' && r <= '9' }) >= 0
}

func (s *Service) Register(ctx context.Context, req domain.RegisterRequest) (*domain.User, error) {
	username := normalizeUsername(req.Username)
	email := normalizeEmail(req.Email)
	name := strings.Join(strings.Fields(req.Name), " ")
	groupName := strings.TrimSpace(req.GroupName)

	switch {
	case !usernamePattern.MatchString(username):
		return nil, fmt.Errorf("%w: username должен содержать 3–32 символа (латиница, цифры, подчёркивание)", core_errors.ErrValidation)
	case name == "" || len(name) > 100:
		return nil, fmt.Errorf("%w: укажите имя", core_errors.ErrValidation)
	case !emailPattern.MatchString(email) || len(email) > 255:
		return nil, fmt.Errorf("%w: введите корректный адрес электронной почты", core_errors.ErrValidation)
	case !validPassword(req.Password):
		return nil, fmt.Errorf("%w: пароль должен содержать минимум 8 символов и хотя бы одну цифру", core_errors.ErrValidation)
	case !domain.IsAcademicGroup(groupName):
		return nil, fmt.Errorf("%w: выберите учебную группу из списка Т-491–Т-495", core_errors.ErrValidation)
	}

	if _, err := s.repo.FindByUsername(ctx, username); err == nil {
		return nil, fmt.Errorf("%w: пользователь с таким username уже существует", core_errors.ErrDuplicateUser)
	} else if !errors.Is(err, core_errors.ErrUserNotFound) {
		return nil, err
	}
	if _, err := s.repo.FindByEmail(ctx, email); err == nil {
		return nil, fmt.Errorf("%w: пользователь с такой почтой уже существует", core_errors.ErrDuplicateUser)
	} else if !errors.Is(err, core_errors.ErrUserNotFound) {
		return nil, err
	}

	groupID, err := s.repo.FindGroupByName(ctx, groupName)
	if err != nil {
		if errors.Is(err, core_errors.ErrGroupNotFound) {
			return nil, fmt.Errorf("%w: выберите учебную группу из списка Т-491–Т-495", core_errors.ErrValidation)
		}
		return nil, err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, errors.New("failed to hash password")
	}

	user := &domain.User{
		Username:     username,
		Name:         name,
		Email:        email,
		PasswordHash: string(hashedPassword),
		GroupName:    groupName,
	}

	// Пользователь и связь с группой создаются одним INSERT.
	if err := s.repo.CreateUser(ctx, user, groupID); err != nil {
		if errors.Is(err, core_errors.ErrDuplicateUser) {
			return nil, fmt.Errorf("%w: пользователь с таким username или почтой уже существует", core_errors.ErrDuplicateUser)
		}
		return nil, err
	}

	user.PasswordHash = ""
	return user, nil
}

func (s *Service) Login(ctx context.Context, req domain.LoginRequest) (*domain.AuthResponse, error) {
	identifier := normalizeUsername(req.Email)
	if identifier == "" || req.Password == "" {
		return nil, fmt.Errorf("%w: введите почту или username и пароль", core_errors.ErrValidation)
	}

	user, err := s.repo.FindByIdentifier(ctx, identifier)
	if err != nil {
		if errors.Is(err, core_errors.ErrUserNotFound) {
			return nil, core_errors.ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		return nil, core_errors.ErrInvalidCredentials
	}

	token, err := s.generateToken(user.ID)
	if err != nil {
		return nil, errors.New("failed to generate token")
	}

	publicUser := *user
	publicUser.PasswordHash = ""
	return &domain.AuthResponse{Token: token, User: &publicUser}, nil
}

func (s *Service) generateToken(userID uuid.UUID) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id": userID.String(),
		"exp":     now.Add(30 * 24 * time.Hour).Unix(),
		"iat":     now.Unix(),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(s.jwtSecret))
}
