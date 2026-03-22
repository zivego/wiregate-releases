package api

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

func (r *Router) limitEnrollmentAttempt(w http.ResponseWriter, req *http.Request) bool {
	key := "enrollment:" + clientAddressKey(req)
	if r.enrollLimiter.allow(key) {
		return true
	}
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many enrollment attempts")
	return false
}

func (r *Router) limitSensitiveAction(w http.ResponseWriter, req *http.Request, actorUserID, action string) bool {
	actorKey := strings.TrimSpace(actorUserID)
	if actorKey == "" {
		actorKey = "addr:" + clientAddressKey(req)
	}
	key := fmt.Sprintf("%s:%s", action, actorKey)
	if r.sensitiveLimiter.allow(key) {
		return true
	}
	writeError(w, http.StatusTooManyRequests, "rate_limited", "too many sensitive action attempts")
	return false
}

func clientAddressKey(req *http.Request) string {
	addr := strings.TrimSpace(req.RemoteAddr)
	if addr == "" {
		return "unknown"
	}
	host, _, err := net.SplitHostPort(addr)
	if err == nil && host != "" {
		return host
	}
	return addr
}
