package users

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"messenger/internal/core/domain"
	middleware "messenger/internal/core/transport/http/middleware"
	core_errors "messenger/internal/errors"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type fakeStore struct {
	users       map[uuid.UUID]domain.User
	lastUpdated domain.UpdateProfileRequest
	searchCalls int
	searchQuery string
}

func (f *fakeStore) GetByID(_ context.Context, id uuid.UUID) (*domain.User, error) {
	user, ok := f.users[id]
	if !ok {
		return nil, core_errors.ErrUserNotFound
	}
	return &user, nil
}

func (f *fakeStore) Search(_ context.Context, exclude uuid.UUID, query string) ([]domain.User, error) {
	f.searchCalls++
	f.searchQuery = query
	result := []domain.User{}
	for _, user := range f.users {
		if user.ID != exclude {
			result = append(result, user)
		}
	}
	return result, nil
}

func (f *fakeStore) UpdateProfile(_ context.Context, id uuid.UUID, req domain.UpdateProfileRequest) (*domain.User, error) {
	f.lastUpdated = req
	user := f.users[id]
	user.Name, user.Username, user.Email = req.Name, req.Username, req.Email
	f.users[id] = user
	return &user, nil
}

func newTestHandler() (*Handler, *fakeStore, uuid.UUID, uuid.UUID) {
	me, other := uuid.New(), uuid.New()
	store := &fakeStore{users: map[uuid.UUID]domain.User{
		me:    {ID: me, Username: "student_a", Name: "Иван Иванов", Email: "a@college.ru", GroupName: "Т-491", CreatedAt: time.Now()},
		other: {ID: other, Username: "student_b", Name: "Пётр Петров", Email: "b@college.ru", GroupName: "Т-492", CreatedAt: time.Now()},
	}}
	return NewHandler(store), store, me, other
}

func request(handler http.Handler, method, path, token string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func authToken(t *testing.T, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{"user_id": userID.String(), "exp": time.Now().Add(time.Hour).Unix()}
	value, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestGetMeRequiresValidTokenAndReturnsGroup(t *testing.T) {
	handler, _, me, _ := newTestHandler()
	withAuth := middleware.AuthMiddleware("secret")(http.HandlerFunc(handler.GetMe))

	rec := request(withAuth, http.MethodGet, "/api/v1/users/me", "", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("no token: status %d", rec.Code)
	}
	rec = request(withAuth, http.MethodGet, "/api/v1/users/me", authToken(t, me), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("valid token: status %d, body %s", rec.Code, rec.Body.String())
	}
	var user domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.GroupName != "Т-491" {
		t.Errorf("group_name = %q", user.GroupName)
	}
	if strings.Contains(rec.Body.String(), "password_hash") {
		t.Error("password_hash leaked into response")
	}
}

func TestGetUserByIDRoutes(t *testing.T) {
	handler, _, me, other := newTestHandler()
	withAuth := middleware.AuthMiddleware("secret")(http.HandlerFunc(handler.GetUser))
	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/users/{userID}", withAuth)

	if rec := request(mux, http.MethodGet, "/api/v1/users/not-a-uuid", authToken(t, me), ""); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid uuid status = %d", rec.Code)
	}
	if rec := request(mux, http.MethodGet, "/api/v1/users/"+uuid.NewString(), authToken(t, me), ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown uuid status = %d", rec.Code)
	}
	rec := request(mux, http.MethodGet, "/api/v1/users/"+other.String(), authToken(t, me), "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "Т-492") {
		t.Fatalf("get user: %d %s", rec.Code, rec.Body.String())
	}
}

func TestSearchExcludesCurrentUserAndProfileIgnoresGroup(t *testing.T) {
	handler, store, me, _ := newTestHandler()
	search := middleware.AuthMiddleware("secret")(http.HandlerFunc(handler.SearchUsers))
	rec := request(search, http.MethodGet, "/api/v1/users/search?q=student", authToken(t, me), "")
	if rec.Code != http.StatusOK {
		t.Fatalf("search status = %d", rec.Code)
	}
	var users []domain.User
	if err := json.Unmarshal(rec.Body.Bytes(), &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID == me {
		t.Fatalf("current user was not excluded: %+v", users)
	}
	// Поиск по @username должен нормализоваться в обычный username.
	rec = request(search, http.MethodGet, "/api/v1/users/search?q=%40student", authToken(t, me), "")
	if rec.Code != http.StatusOK || store.searchQuery != "student" {
		t.Fatalf("search with @: %d query=%q", rec.Code, store.searchQuery)
	}

	update := middleware.AuthMiddleware("secret")(http.HandlerFunc(handler.UpdateProfile))
	body := `{"name":"Новое Имя","username":"student_a","email":"a@college.ru","group_name":"Т-495"}`
	rec = request(update, http.MethodPut, "/api/v1/users/profile", authToken(t, me), body)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d, body %s", rec.Code, rec.Body.String())
	}
	if store.lastUpdated.Name != "Новое Имя" || store.users[me].GroupName != "Т-491" {
		t.Fatalf("group was changed by profile update: %+v", store.users[me])
	}
}
