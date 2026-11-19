// Integration tests run against a real PostgreSQL database. They are skipped
// unless TEST_DB_SOURCE points to a dedicated test database with migrations applied.
package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"messenger/internal/core/domain"
	"messenger/internal/core/features/auth"
	"messenger/internal/core/features/chats"
	"messenger/internal/core/features/messages"
	"messenger/internal/core/features/users"
	middleware "messenger/internal/core/transport/http/middleware"
	"messenger/internal/db"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"
)

const jwtSecret = "integration-test-secret"

func testPool(t *testing.T) *db.Pool {
	t.Helper()
	dsn := os.Getenv("TEST_DB_SOURCE")
	if dsn == "" {
		t.Skip("TEST_DB_SOURCE is not set: skipping PostgreSQL integration tests")
	}
	pool, err := db.NewPool(context.Background(), dsn, zap.NewNop())
	if err != nil {
		t.Fatalf("connect to test DB: %v", err)
	}
	t.Cleanup(pool.Close)
	_, err = pool.Exec(context.Background(), `
		TRUNCATE TABLE messages, chat_members, chats, users, academic_groups RESTART IDENTITY CASCADE`)
	if err != nil {
		t.Fatalf("cleanup test DB: %v", err)
	}
	_, err = pool.Exec(context.Background(), `
		INSERT INTO academic_groups (name) VALUES ('Т-491'),('Т-492'),('Т-493'),('Т-494'),('Т-495')
		ON CONFLICT (name) DO NOTHING`)
	if err != nil {
		t.Fatalf("seed academic groups: %v", err)
	}
	return pool
}

func doJSON(t *testing.T, handler http.Handler, method, path, token string, payload interface{}) *httptest.ResponseRecorder {
	t.Helper()
	var body []byte
	if payload != nil {
		var err error
		body, err = json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
	}
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func decode[T any](t *testing.T, rec *httptest.ResponseRecorder) T {
	t.Helper()
	var result T
	if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
		t.Fatalf("decode %s: %v", rec.Body.String(), err)
	}
	return result
}

func register(t *testing.T, service *auth.Service, username, group string) domain.User {
	t.Helper()
	user, err := service.Register(context.Background(), domain.RegisterRequest{
		Username:  username,
		Name:      "Студент " + username,
		Email:     username + "@college.ru",
		Password:  "password1",
		GroupName: group,
	})
	if err != nil {
		t.Fatalf("register %s: %v", username, err)
	}
	return *user
}

func login(t *testing.T, service *auth.Service, identifier string) string {
	t.Helper()
	resp, err := service.Login(context.Background(), domain.LoginRequest{Email: identifier, Password: "password1"})
	if err != nil {
		t.Fatalf("login %s: %v", identifier, err)
	}
	return resp.Token
}

func TestHTTPAuthRegistrationGroupsLoginAndDuplicates(t *testing.T) {
	pool := testPool(t)
	authHandler := auth.NewHandler(auth.NewService(auth.NewRepository(pool), jwtSecret))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	for i, group := range domain.AcademicGroups {
		username := fmt.Sprintf("user_group%d", i+1)
		rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/register", "", domain.RegisterRequest{
			Username: username, Name: "Студент " + group, Email: username + "@college.ru",
			Password: "password1", GroupName: group,
		})
		if rec.Code != http.StatusCreated {
			t.Fatalf("register %s: status %d body %s", group, rec.Code, rec.Body.String())
		}
		user := decode[domain.User](t, rec)
		if user.GroupName != group {
			t.Errorf("response group = %q, want %q", user.GroupName, group)
		}
		var storedGroup string
		var hash string
		err := pool.QueryRow(context.Background(), `
			SELECT ag.name, u.password_hash FROM users u JOIN academic_groups ag ON ag.id = u.group_id
			WHERE u.id = $1`, user.ID).Scan(&storedGroup, &hash)
		if err != nil {
			t.Fatalf("group not persisted for %s: %v", group, err)
		}
		if storedGroup != group {
			t.Errorf("DB group = %q, want %q", storedGroup, group)
		}
		if !strings.HasPrefix(hash, "$2") || bcrypt.CompareHashAndPassword([]byte(hash), []byte("password1")) != nil {
			t.Error("password is not a bcrypt hash of the submitted password")
		}
	}

	for i, group := range []string{"", "ИС-21", "Преподаватель"} {
		rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/register", "", domain.RegisterRequest{
			Username: fmt.Sprintf("bad_group%d", i), Name: "Студент",
			Email: fmt.Sprintf("bad%d@college.ru", i), Password: "password1", GroupName: group,
		})
		if rec.Code != http.StatusBadRequest {
			t.Errorf("group %q: status %d body %s", group, rec.Code, rec.Body.String())
		}
	}

	duplicate := domain.RegisterRequest{
		Username: "USER_GROUP1", Name: "Дубль", Email: "other@college.ru",
		Password: "password1", GroupName: "Т-491",
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/register", "", duplicate); rec.Code != http.StatusConflict {
		t.Errorf("duplicate username: status %d body %s", rec.Code, rec.Body.String())
	}
	duplicate.Username = "other_user"
	duplicate.Email = "USER_GROUP1@COLLEGE.RU"
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/register", "", duplicate); rec.Code != http.StatusConflict {
		t.Errorf("duplicate email: status %d body %s", rec.Code, rec.Body.String())
	}

	for _, identifier := range []string{"user_group1@college.ru", "USER_GROUP1"} {
		rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
			"email": identifier, "password": "password1",
		})
		if rec.Code != http.StatusOK {
			t.Fatalf("login by %s: status %d body %s", identifier, rec.Code, rec.Body.String())
		}
		response := decode[domain.AuthResponse](t, rec)
		if response.Token == "" || response.User.GroupName != "Т-491" {
			t.Fatalf("unexpected login response: %+v", response)
		}
	}
	rec := doJSON(t, mux, http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": "user_group1@college.ru", "password": "wrongpassword1",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("wrong password: status %d body %s", rec.Code, rec.Body.String())
	}
}

func TestHTTPUsersChatsMessagesAndStaticFiles(t *testing.T) {
	pool := testPool(t)
	authService := auth.NewService(auth.NewRepository(pool), jwtSecret)
	alice := register(t, authService, "alice", "Т-491")
	bob := register(t, authService, "bob", "Т-492")
	register(t, authService, "outsider", "Т-493")

	usersHandler := users.NewHandler(users.NewRepository(pool))
	chatsService := chats.NewService(chats.NewRepository(pool))
	chatsHandler := chats.NewHandler(chatsService)
	messagesService := messages.NewService(messages.NewRepository(pool), nil, "test")
	messagesHandler := messages.NewHandler(messagesService)
	authMW := middleware.AuthMiddleware(jwtSecret)

	mux := http.NewServeMux()
	mux.Handle("GET /api/v1/users/me", authMW(http.HandlerFunc(usersHandler.GetMe)))
	mux.Handle("GET /api/v1/users/search", authMW(http.HandlerFunc(usersHandler.SearchUsers)))
	mux.Handle("GET /api/v1/users/{userID}", authMW(http.HandlerFunc(usersHandler.GetUser)))
	mux.Handle("GET /api/v1/chats/", authMW(http.HandlerFunc(chatsHandler.GetMyChats)))
	mux.Handle("POST /api/v1/chats/private", authMW(http.HandlerFunc(chatsHandler.CreatePrivateChat)))
	mux.Handle("GET /api/v1/chats/{chatID}/messages", authMW(http.HandlerFunc(messagesHandler.GetMessages)))
	mux.Handle("POST /api/v1/chats/{chatID}/messages", authMW(http.HandlerFunc(messagesHandler.CreateMessage)))
	mux.Handle("POST /api/v1/chats/{chatID}/read", authMW(http.HandlerFunc(messagesHandler.MarkAsRead)))

	aliceToken := login(t, authService, "alice")
	bobToken := login(t, authService, "bob")
	outsiderToken := login(t, authService, "outsider")

	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/users/me", "", nil); rec.Code != http.StatusUnauthorized {
		t.Fatalf("/users/me without token: %d", rec.Code)
	}
	rec := doJSON(t, mux, http.MethodGet, "/api/v1/users/me", aliceToken, nil)
	me := decode[domain.User](t, rec)
	if rec.Code != http.StatusOK || me.GroupName != "Т-491" || strings.Contains(rec.Body.String(), "password_hash") {
		t.Fatalf("/users/me: %d %s", rec.Code, rec.Body.String())
	}

	chatPath := "/api/v1/chats/private"
	first := doJSON(t, mux, http.MethodPost, chatPath, aliceToken, domain.CreatePrivateChatRequest{UserID: bob.ID})
	if first.Code != http.StatusCreated {
		t.Fatalf("create private chat: %d %s", first.Code, first.Body.String())
	}
	chat := decode[domain.Chat](t, first)
	second := doJSON(t, mux, http.MethodPost, chatPath, bobToken, domain.CreatePrivateChatRequest{UserID: alice.ID})
	reused := decode[domain.Chat](t, second)
	if reused.ID != chat.ID {
		t.Fatalf("private chat not reused: %s vs %s", chat.ID, reused.ID)
	}
	var chatCount int
	if err := pool.QueryRow(context.Background(), `SELECT COUNT(*) FROM chats WHERE type = 'private'`).Scan(&chatCount); err != nil || chatCount != 1 {
		t.Fatalf("duplicate private chats: count=%d err=%v", chatCount, err)
	}
	if rec := doJSON(t, mux, http.MethodPost, chatPath, aliceToken, domain.CreatePrivateChatRequest{UserID: alice.ID}); rec.Code != http.StatusBadRequest {
		t.Errorf("self chat: %d %s", rec.Code, rec.Body.String())
	}

	messagesPath := "/api/v1/chats/" + chat.ID.String() + "/messages"
	if rec := doJSON(t, mux, http.MethodGet, messagesPath, outsiderToken, nil); rec.Code != http.StatusForbidden {
		t.Errorf("outsider history: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, messagesPath, outsiderToken, domain.CreateMessageRequest{Content: "hack"}); rec.Code != http.StatusForbidden {
		t.Errorf("outsider send: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodPost, "/api/v1/chats/"+chat.ID.String()+"/read", outsiderToken, nil); rec.Code != http.StatusForbidden {
		t.Errorf("outsider read mark: %d %s", rec.Code, rec.Body.String())
	}

	sent := doJSON(t, mux, http.MethodPost, messagesPath, aliceToken, domain.CreateMessageRequest{Content: "Привет, Боб!"})
	if sent.Code != http.StatusCreated {
		t.Fatalf("send message: %d %s", sent.Code, sent.Body.String())
	}
	sentMessage := decode[domain.Message](t, sent)
	if sentMessage.SenderName == "" {
		t.Error("POST response has empty sender_name")
	}

	// История переживает "перезагрузку страницы" — новый запрос с тем же токеном.
	history := doJSON(t, mux, http.MethodGet, messagesPath, bobToken, nil)
	messagesList := decode[[]domain.Message](t, history)
	if history.Code != http.StatusOK || len(messagesList) != 1 {
		t.Fatalf("history: %d %s", history.Code, history.Body.String())
	}
	if messagesList[0].SenderName != "Студент alice" || messagesList[0].ID != sentMessage.ID {
		t.Fatalf("sender_name/history mismatch: %+v", messagesList[0])
	}

	list := doJSON(t, mux, http.MethodGet, "/api/v1/chats/", aliceToken, nil)
	var items []map[string]interface{}
	if err := json.Unmarshal(list.Body.Bytes(), &items); err != nil {
		t.Fatalf("chat list JSON: %v (%s)", err, list.Body.String())
	}
	if len(items) == 0 {
		t.Fatal("chat list is empty")
	}
	foundPrivate := false
	for _, item := range items {
		// Базовые поля обязательны для всех элементов (включая «Избранное»).
		for _, field := range []string{"id", "type", "name", "unread_count", "created_at"} {
			if _, ok := item[field]; !ok {
				t.Errorf("chat list item misses %q: %s", field, list.Body.String())
			}
		}
		if item["type"] != "private" {
			continue
		}
		for _, field := range []string{"partner_id", "last_message", "last_message_time", "last_sender_id"} {
			if _, ok := item[field]; !ok {
				t.Errorf("private chat item misses %q: %s", field, list.Body.String())
			}
		}
		if item["partner_id"] == bob.ID.String() && item["name"] == "Студент bob" {
			foundPrivate = true
		}
	}
	if !foundPrivate {
		t.Errorf("private chat name/partner are wrong: %s", list.Body.String())
	}

	public := doJSON(t, mux, http.MethodGet, "/api/v1/users/"+bob.ID.String(), aliceToken, nil)
	if public.Code != http.StatusOK || !strings.Contains(public.Body.String(), "Т-492") {
		t.Errorf("GET /users/{id}: %d %s", public.Code, public.Body.String())
	}
	if rec := doJSON(t, mux, http.MethodGet, "/api/v1/users/"+uuid.NewString(), aliceToken, nil); rec.Code != http.StatusNotFound {
		t.Errorf("unknown user: %d", rec.Code)
	}
	search := doJSON(t, mux, http.MethodGet, "/api/v1/users/search?q=alice", aliceToken, nil)
	var found []domain.User
	_ = json.Unmarshal(search.Body.Bytes(), &found)
	for _, user := range found {
		if user.ID == alice.ID {
			t.Error("search returned the current user")
		}
	}

	staticMux := http.NewServeMux()
	staticMux.Handle("/", http.FileServer(http.Dir("../../web")))
	rec = doJSON(t, staticMux, http.MethodGet, "/", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("static index: %d", rec.Code)
	}
}
