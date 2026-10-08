package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"net/http"
	"strings"
	"time"
)

const (
	maxBodyBytes  = 1 << 20 // 1 MiB; uploads use dedicated endpoints
	headerRequest = "X-Request-ID"
)

type ctxKey int

const (
	requestIDKey ctxKey = iota
	userKey
)

// requestID assigns every request an ID, echoed in responses and logs.
// Client-supplied IDs are ignored to avoid log injection.
func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newID()
		w.Header().Set(headerRequest, id)
		ctx := context.WithValue(r.Context(), requestIDKey, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// securityHeaders applies defaults. API responses allow nothing; UI pages get
// a CSP that permits only same-origin scripts and styles (no inline code).
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Strict-Transport-Security", "max-age=31536000")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
			h.Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		} else {
			h.Set("Content-Security-Policy",
				"default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; "+
					"connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		}
		next.ServeHTTP(w, r)
	})
}

// limitBody caps request bodies so a client cannot exhaust memory.
func limitBody(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)
		next.ServeHTTP(w, r)
	})
}

// recoverPanic converts panics into a generic 500 and logs the reason.
func recoverPanic(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					log.Error("panic recovered",
						"request_id", requestIDFrom(r),
						"operation", r.Method+" "+r.URL.Path,
						"error_code", ErrInternal.Code,
						"panic", rec,
					)
					writeError(w, requestIDFrom(r), ErrInternal)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// accessLog records method, path, status and latency. The query string is
// omitted because it can carry tokens.
func accessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			log.Info("request",
				"request_id", requestIDFrom(r),
				"method", r.Method,
				"path", r.URL.Path,
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
			)
		})
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func requestIDFrom(r *http.Request) string {
	if v, ok := r.Context().Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

func newID() string {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "unavailable"
	}
	return hex.EncodeToString(b)
}
