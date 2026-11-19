package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStaticFrontendIsServedFromWebDirectory(t *testing.T) {
	mux := http.NewServeMux()
	webFS := http.FileServer(http.Dir("../../web"))
	mux.Handle("/", webFS)

	for _, path := range []string{
		"/", "/index.html", "/login.html", "/messages.html", "/contacts.html",
		"/groups.html", "/files.html", "/settings.html", "/api.js", "/app.js", "/style.css",
	} {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			// FileServer канонизирует /index.html в / (301).
			if rec.Code != http.StatusOK && !(path == "/index.html" && rec.Code == http.StatusMovedPermanently) {
				t.Fatalf("status = %d", rec.Code)
			}
			if rec.Code == http.StatusOK && rec.Body.Len() == 0 {
				t.Error("empty response body")
			}
		})
	}
}

func TestAPIRoutesTakePrecedenceOverFileServer(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/auth/login", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
	})
	mux.Handle("GET /api/v1/users/me", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	mux.Handle("/", http.FileServer(http.Dir("../../web")))

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil))
	if rec.Code != http.StatusTeapot {
		t.Fatalf("login route status = %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/users/me", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("me route status = %d", rec.Code)
	}
}
