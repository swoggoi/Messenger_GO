package http

import (
	"net/http"
	"net/url"
	"strings"
)

// OriginAllowed accepts same-origin requests plus an explicit allowlist from
// CORS_ALLOWED_ORIGINS. Wildcard CORS is not safe with bearer JWTs.
func OriginAllowed(origin string, r *http.Request, allowedOrigins []string) bool {
	if origin == "" {
		return true
	}
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return false
	}
	if strings.EqualFold(parsed.Host, r.Host) {
		return true
	}
	for _, allowed := range allowedOrigins {
		allowed = strings.TrimSpace(allowed)
		if allowed != "" && strings.EqualFold(origin, allowed) {
			return true
		}
	}
	return false
}

func CORS(next http.Handler, allowedOrigins []string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		w.Header().Add("Vary", "Origin")
		if origin != "" {
			if !OriginAllowed(origin, r, allowedOrigins) {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Content-Type, Content-Length, Authorization")
		w.Header().Set("Access-Control-Max-Age", "600")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
