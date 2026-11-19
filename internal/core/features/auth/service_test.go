package auth

import (
	"context"
	"errors"
	"strings"
	"testing"

	"messenger/internal/core/domain"
	core_errors "messenger/internal/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type fakeStore struct {
	users  []*domain.User
	groups map[string]uuid.UUID
}

func newFakeStore() *fakeStore {
	groups := map[string]uuid.UUID{}
	for _, name := range domain.AcademicGroups {
		groups[name] = uuid.New()
	}
	return &fakeStore{groups: groups}
}

func (f *fakeStore) CreateUser(_ context.Context, user *domain.User, groupID uuid.UUID) error {
	if groupID == uuid.Nil {
		return errors.New("group_id is required")
	}
	for _, existing := range f.users {
		if strings.EqualFold(existing.Username, user.Username) || strings.EqualFold(existing.Email, user.Email) {
			return core_errors.ErrDuplicateUser
		}
	}
	copy := *user
	copy.ID = uuid.New()
	f.users = append(f.users, &copy)
	*user = copy
	return nil
}

func (f *fakeStore) FindByIdentifier(_ context.Context, identifier string) (*domain.User, error) {
	for _, user := range f.users {
		if strings.EqualFold(user.Email, identifier) || strings.EqualFold(user.Username, identifier) {
			return user, nil
		}
	}
	return nil, core_errors.ErrUserNotFound
}

func (f *fakeStore) FindByEmail(_ context.Context, email string) (*domain.User, error) {
	for _, user := range f.users {
		if strings.EqualFold(user.Email, email) {
			return user, nil
		}
	}
	return nil, core_errors.ErrUserNotFound
}

func (f *fakeStore) FindByUsername(_ context.Context, username string) (*domain.User, error) {
	for _, user := range f.users {
		if strings.EqualFold(user.Username, username) {
			return user, nil
		}
	}
	return nil, core_errors.ErrUserNotFound
}

func (f *fakeStore) FindGroupByName(_ context.Context, name string) (uuid.UUID, error) {
	if id, ok := f.groups[name]; ok {
		return id, nil
	}
	return uuid.Nil, core_errors.ErrGroupNotFound
}

func registerPayload(username, email, group string) domain.RegisterRequest {
	return domain.RegisterRequest{
		Username:  username,
		Name:      "Иван Иванов",
		Email:     email,
		Password:  "password1",
		GroupName: group,
	}
}

func TestRegisterAcceptsEveryAcademicGroup(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, "test-secret")
	for i, group := range domain.AcademicGroups {
		req := registerPayload("student_"+string(rune('a'+i)), "student"+string(rune('a'+i))+"@college.ru", group)
		user, err := service.Register(context.Background(), req)
		if err != nil {
			t.Fatalf("register %s: %v", group, err)
		}
		if user.GroupName != group {
			t.Errorf("group_name = %q, want %q", user.GroupName, group)
		}
		if user.PasswordHash != "" {
			t.Error("password hash leaked into response")
		}
		stored := store.users[i]
		if stored.PasswordHash == "" || bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte(req.Password)) != nil {
			t.Error("password was not stored as a valid bcrypt hash")
		}
		if stored.Username != req.Username || stored.Email != req.Email {
			t.Error("username/email were not normalised consistently")
		}
	}
}

func TestRegisterRejectsMissingOrUnknownGroup(t *testing.T) {
	for _, group := range []string{"", "ИС-21", "Преподаватель"} {
		service := NewService(newFakeStore(), "test-secret")
		req := registerPayload("student_x", "studentx@college.ru", group)
		_, err := service.Register(context.Background(), req)
		if !errors.Is(err, core_errors.ErrValidation) {
			t.Errorf("group %q: got %v, want validation error", group, err)
		}
	}
}

func TestRegisterRejectsDuplicateUsernameAndEmail(t *testing.T) {
	service := NewService(newFakeStore(), "test-secret")
	if _, err := service.Register(context.Background(), registerPayload("student_a", "a@college.ru", "Т-491")); err != nil {
		t.Fatal(err)
	}
	cases := []domain.RegisterRequest{
		registerPayload("STUDENT_A", "other@college.ru", "Т-492"),
		registerPayload("student_b", "A@college.ru", "Т-492"),
	}
	for _, req := range cases {
		if _, err := service.Register(context.Background(), req); !errors.Is(err, core_errors.ErrDuplicateUser) {
			t.Errorf("duplicate register: got %v, want duplicate error", err)
		}
	}
}

func TestRegisterValidatesPassword(t *testing.T) {
	for _, password := range []string{"short1", "password"} {
		service := NewService(newFakeStore(), "test-secret")
		req := registerPayload("student_p", "p@college.ru", "Т-491")
		req.Password = password
		if _, err := service.Register(context.Background(), req); !errors.Is(err, core_errors.ErrValidation) {
			t.Errorf("password %q: got %v, want validation error", password, err)
		}
	}
}

func TestLoginByEmailAndUsername(t *testing.T) {
	store := newFakeStore()
	service := NewService(store, "test-secret")
	req := registerPayload("n_orlov", "n.orlov@college.ru", "Т-491")
	if _, err := service.Register(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	for _, identifier := range []string{"n.orlov@college.ru", "N_ORLOV", " n_orlov ", "@n_orlov", "@N_ORLOV"} {
		resp, err := service.Login(context.Background(), domain.LoginRequest{Email: identifier, Password: req.Password})
		if err != nil {
			t.Fatalf("login %q: %v", identifier, err)
		}
		if resp.Token == "" || resp.User == nil || resp.User.GroupName != "Т-491" {
			t.Fatalf("unexpected login response: %+v", resp)
		}
		token, err := jwt.Parse(resp.Token, func(*jwt.Token) (interface{}, error) { return []byte("test-secret"), nil },
			jwt.WithValidMethods([]string{"HS256"}))
		if err != nil || !token.Valid {
			t.Fatalf("invalid JWT: %v", err)
		}
	}
}

func TestLoginRejectsWrongPassword(t *testing.T) {
	service := NewService(newFakeStore(), "test-secret")
	req := registerPayload("n_orlov", "n.orlov@college.ru", "Т-491")
	if _, err := service.Register(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	_, err := service.Login(context.Background(), domain.LoginRequest{Email: req.Email, Password: "wrongpassword1"})
	if !errors.Is(err, core_errors.ErrInvalidCredentials) {
		t.Fatalf("got %v, want invalid credentials", err)
	}
}
