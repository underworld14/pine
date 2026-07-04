package server

import (
	"net"
	"net/http"
	"net/url"
	"strings"
)

// securityMiddleware defends the no-auth localhost API against DNS-rebinding and
// drive-by cross-origin requests: the Host header must name localhost, and any
// state-changing request carrying an Origin must have a localhost origin. No
// CORS headers are ever sent (default-deny for browsers).
func securityMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowedHost(r.Host) {
			http.Error(w, "forbidden: unexpected Host header", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			if o := r.Header.Get("Origin"); o != "" && !allowedOrigin(o) {
				http.Error(w, "forbidden: cross-origin request", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func allowedHost(host string) bool {
	h := host
	if hh, _, err := net.SplitHostPort(host); err == nil {
		h = hh
	}
	h = strings.TrimSuffix(strings.TrimPrefix(h, "["), "]")
	switch h {
	case "localhost", "127.0.0.1", "::1":
		return true
	}
	return false
}

func allowedOrigin(origin string) bool {
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" {
		return false
	}
	return allowedHost(u.Host)
}
