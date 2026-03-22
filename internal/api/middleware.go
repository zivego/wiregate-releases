package api

import (
	"net/http"
	"strings"
)

const sessionCookieName = "wiregate_session"

// extractToken reads the bearer token from the Authorization header first,
// then falls back to the wiregate_session cookie.
func extractToken(req *http.Request) (string, bool) {
	if auth := req.Header.Get("Authorization"); auth != "" {
		const prefix = "Bearer "
		if strings.HasPrefix(auth, prefix) {
			token := strings.TrimPrefix(auth, prefix)
			if token != "" {
				return token, true
			}
		}
	}

	if cookie, err := req.Cookie(sessionCookieName); err == nil && cookie.Value != "" {
		return cookie.Value, true
	}

	return "", false
}
