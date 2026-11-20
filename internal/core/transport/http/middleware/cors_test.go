package http_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	middleware "messenger/internal/core/transport/http/middleware"
)

func TestOriginAllowed(t *testing.T) {
	request := httptest.NewRequest(http.MethodGet, "http://localhost:8080/", nil)

	tests := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{"no origin", "", nil, true},
		{"same origin", "http://localhost:8080", nil, true},
		{"same origin case insensitive", "http://LOCALHOST:8080", nil, true},
		{"exact allowlist", "https://example.com", []string{"https://example.com"}, true},
		{"exact allowlist miss", "https://evil.com", []string{"https://example.com"}, false},
		{
			"cloudflare quick tunnel",
			"https://moon-star-123.trycloudflare.com",
			[]string{"https://*.trycloudflare.com"},
			true,
		},
		{
			"nested subdomain",
			"https://a.b.trycloudflare.com",
			[]string{"https://*.trycloudflare.com"},
			true,
		},
		{
			"wildcard requires at least one label",
			"https://trycloudflare.com",
			[]string{"https://*.trycloudflare.com"},
			false,
		},
		{
			"wildcard is not a suffix match",
			"https://eviltrycloudflare.com",
			[]string{"https://*.trycloudflare.com"},
			false,
		},
		{
			"wildcard enforces scheme",
			"http://abc.trycloudflare.com",
			[]string{"https://*.trycloudflare.com"},
			false,
		},
		{
			"scheme-less wildcard",
			"https://abc.trycloudflare.com",
			[]string{"*.trycloudflare.com"},
			true,
		},
		{
			"wildcard with port",
			"https://abc.example.com:8443",
			[]string{"https://*.example.com:8443"},
			true,
		},
		{
			"wildcard with wrong port",
			"https://abc.example.com:9443",
			[]string{"https://*.example.com:8443"},
			false,
		},
		{
			"blanket wildcard rejected",
			"https://evil.com",
			[]string{"*"},
			false,
		},
		{
			"wildcard in the middle rejected",
			"https://abc.example.com.evil.com",
			[]string{"https://example.*.com"},
			false,
		},
		{"malformed origin", "not a url", []string{"https://*.trycloudflare.com"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := middleware.OriginAllowed(tt.origin, request, tt.allowed)
			if got != tt.want {
				t.Fatalf("OriginAllowed(%q, %v) = %v, want %v", tt.origin, tt.allowed, got, tt.want)
			}
		})
	}
}
