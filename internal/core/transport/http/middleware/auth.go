package http

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type contextKey string

const UserIDKey contextKey = "user_id"

func writeUnauthorized(w http.ResponseWriter, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// AuthMiddleware validates the JWT on every protected operation. Frontend
// redirects are never trusted as an authorisation boundary.
func AuthMiddleware(jwtSecret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			header := r.Header.Get("Authorization")
			parts := strings.SplitN(header, " ", 2)
			if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
				writeUnauthorized(w, "authorization header required")
				return
			}

			claims := jwt.MapClaims{}
			token, err := jwt.ParseWithClaims(strings.TrimSpace(parts[1]), claims,
				func(token *jwt.Token) (interface{}, error) {
					return []byte(jwtSecret), nil
				},
				jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
			)
			if err != nil || !token.Valid {
				writeUnauthorized(w, "invalid token")
				return
			}

			userID, err := parseUserID(claims["user_id"])
			if err != nil || userID == uuid.Nil {
				writeUnauthorized(w, "invalid token claims")
				return
			}

			ctx := context.WithValue(r.Context(), UserIDKey, userID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func parseUserID(claim interface{}) (uuid.UUID, error) {
	switch v := claim.(type) {
	case string:
		return uuid.Parse(v)
	case uuid.UUID:
		return v, nil
	default:
		return uuid.Nil, jwt.ErrTokenInvalidClaims
	}
}
