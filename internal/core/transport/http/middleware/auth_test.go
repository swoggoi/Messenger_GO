package http_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	middleware "messenger/internal/core/transport/http/middleware"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func token(t *testing.T, secret string, userID uuid.UUID) string {
	t.Helper()
	claims := jwt.MapClaims{"user_id": userID.String(), "exp": time.Now().Add(time.Hour).Unix()}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatal(err)
	}
	return signed
}

func TestAuthMiddleware(t *testing.T) {
	const secret = "unit-test-secret"
	wantID := uuid.New()
	protected := middleware.AuthMiddleware(secret)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := r.Context().Value(middleware.UserIDKey)
		if got != wantID {
			t.Errorf("context user id = %v, want %v", got, wantID)
		}
		w.WriteHeader(http.StatusNoContent)
	}))

	cases := []struct {
		name   string
		header string
		status int
	}{
		{"no header", "", http.StatusUnauthorized},
		{"bad format", "Basic abc", http.StatusUnauthorized},
		{"bad signature", "Bearer " + token(t, "other-secret", wantID), http.StatusUnauthorized},
		{"valid", "Bearer " + token(t, secret, wantID), http.StatusNoContent},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			protected.ServeHTTP(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d; body %s", rec.Code, tc.status, rec.Body.String())
			}
			if tc.status == http.StatusUnauthorized {
				var body map[string]string
				if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || body["error"] == "" {
					t.Errorf("expected JSON error, got %s", rec.Body.String())
				}
			}
		})
	}
}

func TestCORSRejectsForeignOrigin(t *testing.T) {
	handler := middleware.CORS(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), []string{"https://allowed.example"})

	req := httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/users/me", nil)
	req.Header.Set("Origin", "https://evil.example")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin status = %d, want 403", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Error("wildcard CORS header must not be sent")
	}

	req = httptest.NewRequest(http.MethodGet, "http://localhost:8080/api/v1/users/me", nil)
	req.Header.Set("Origin", "http://localhost:8080")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:8080" {
		t.Fatalf("same-origin request rejected: %d %v", rec.Code, rec.Header())
	}
}
