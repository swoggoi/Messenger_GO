package http

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// OriginAllowed accepts same-origin requests plus exact origins or subdomain
// wildcards from CORS_ALLOWED_ORIGINS, e.g. https://*.trycloudflare.com.
// A wildcard is only valid as the first DNS label; a blanket `*` is never
// allowed because that would be unsafe with bearer JWTs.
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
		if originMatchesAllowlist(origin, parsed, allowed) {
			return true
		}
	}
	return false
}

// originMatchesAllowlist matches an origin against one CORS_ALLOWED_ORIGINS entry:
//   - https://example.com   — exact origin
//   - https://*.example.com — any subdomain, HTTPS only
//   - *.example.com         — any subdomain, any scheme
func originMatchesAllowlist(origin string, parsed *url.URL, allowed string) bool {
	allowed = strings.ToLower(strings.TrimSpace(allowed))
	if allowed == "" {
		return false
	}
	if !strings.Contains(allowed, "*") {
		return strings.EqualFold(origin, allowed)
	}

	scheme := ""
	pattern := allowed
	if idx := strings.Index(pattern, "://"); idx >= 0 {
		scheme = pattern[:idx]
		pattern = pattern[idx+3:]
		if strings.Contains(scheme, "*") {
			return false
		}
	}
	if !strings.HasPrefix(pattern, "*.") || strings.Contains(pattern[2:], "*") {
		return false
	}
	if scheme != "" && !strings.EqualFold(parsed.Scheme, scheme) {
		return false
	}

	base := pattern[2:]
	baseHost := base
	basePort := ""
	if h, port, err := net.SplitHostPort(base); err == nil {
		baseHost, basePort = h, port
	}
	if basePort != "" && parsed.Port() != basePort {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	suffix := "." + baseHost
	if host == baseHost || !strings.HasSuffix(host, suffix) {
		return false
	}
	return strings.TrimSuffix(host, suffix) != ""
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
