package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strings"
)

const (
	AuthTokenQueryParam = "openmessage_token"
	AuthTokenHeader     = "X-OpenMessage-Token"
	authCookieName      = "openmessage_token"
)

func NewAuthToken() (string, error) {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(b[:]), nil
}

func SecureHandler(next http.Handler, authToken string) http.Handler {
	authToken = strings.TrimSpace(authToken)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w)

		if authToken == "" {
			next.ServeHTTP(w, r)
			return
		}

		queryToken := strings.TrimSpace(r.URL.Query().Get(AuthTokenQueryParam))
		if tokenMatches(queryToken, authToken) {
			setAuthCookie(w, authToken)
			if shouldCleanTokenFromURL(r) {
				http.Redirect(w, r, cleanTokenURL(r.URL), http.StatusSeeOther)
				return
			}
		}

		if requiresAuth(r.URL.Path) {
			if !requestAuthorized(r, authToken) {
				http.Error(w, "missing or invalid OpenMessage auth token", http.StatusUnauthorized)
				return
			}
			if isStateChangingMethod(r.Method) && !sameOriginRequest(r) {
				http.Error(w, "cross-origin state-changing request blocked", http.StatusForbidden)
				return
			}
		}

		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func requiresAuth(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/") ||
		path == "/mcp" || strings.HasPrefix(path, "/mcp/")
}

func requestAuthorized(r *http.Request, authToken string) bool {
	if tokenMatches(bearerToken(r.Header.Get("Authorization")), authToken) {
		return true
	}
	if tokenMatches(r.Header.Get(AuthTokenHeader), authToken) {
		return true
	}
	if tokenMatches(r.URL.Query().Get(AuthTokenQueryParam), authToken) {
		return true
	}
	cookie, err := r.Cookie(authCookieName)
	return err == nil && tokenMatches(cookie.Value, authToken)
}

func tokenMatches(got, want string) bool {
	got = strings.TrimSpace(got)
	want = strings.TrimSpace(want)
	if got == "" || want == "" || len(got) != len(want) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearerToken(header string) string {
	header = strings.TrimSpace(header)
	if len(header) < len("Bearer ") || !strings.EqualFold(header[:len("Bearer ")], "Bearer ") {
		return ""
	}
	return strings.TrimSpace(header[len("Bearer "):])
}

func setAuthCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
}

func shouldCleanTokenFromURL(r *http.Request) bool {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return false
	}
	path := r.URL.Path
	return !requiresAuth(path)
}

func cleanTokenURL(u *url.URL) string {
	cp := *u
	q := cp.Query()
	q.Del(AuthTokenQueryParam)
	cp.RawQuery = q.Encode()
	if cp.RawQuery == "" && cp.Path == "" {
		cp.Path = "/"
	}
	return cp.String()
}

func isStateChangingMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return false
	default:
		return true
	}
}

func sameOriginRequest(r *http.Request) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	return origin == scheme+"://"+r.Host
}
