package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSecureHandlerProtectsAPI(t *testing.T) {
	handler := SecureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "secret-token")

	t.Run("rejects missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusUnauthorized)
		}
	})

	t.Run("accepts token header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
		req.Header.Set(AuthTokenHeader, "secret-token")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
		}
	})

	t.Run("blocks cross-origin post even with cookie", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/send", nil)
		req.Host = "127.0.0.1"
		req.Header.Set("Origin", "http://evil.example")
		req.AddCookie(&http.Cookie{Name: authCookieName, Value: "secret-token"})
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
		}
	})
}

func TestSecureHandlerSetsCookieAndCleansUIURL(t *testing.T) {
	handler := SecureHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}), "secret-token")

	req := httptest.NewRequest(http.MethodGet, "/?"+AuthTokenQueryParam+"=secret-token&x=1", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if got := rec.Header().Get("Location"); got != "/?x=1" {
		t.Fatalf("Location = %q, want /?x=1", got)
	}
	if cookies := rec.Result().Cookies(); len(cookies) != 1 || cookies[0].Name != authCookieName {
		t.Fatalf("auth cookie not set: %#v", cookies)
	}
}
