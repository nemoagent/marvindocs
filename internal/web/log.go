package web

import (
	"log/slog"
	"net/http"
	"time"
)

// statusRecorder tracks the HTTP status code written by a handler so
// the access-log middleware can report it. Without this wrapper we
// would have to assume 200 (the default), which silently hides 304s
// and errors.
type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (s *statusRecorder) WriteHeader(code int) {
	s.status = code
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(p []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	return s.ResponseWriter.Write(p)
}

// AccessLogHandler wraps h with structured request logging:
//
//	method, path, status, dur (ms), remote
//
// Logs at info level for 2xx/3xx, warn for 4xx, error for 5xx.
func AccessLogHandler(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		h.ServeHTTP(rec, r)
		dur := time.Since(start)
		level := slog.LevelInfo
		switch {
		case rec.status >= 500:
			level = slog.LevelError
		case rec.status >= 400:
			level = slog.LevelWarn
		}
		slog.Log(r.Context(), level, "http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", rec.status,
			"dur_ms", dur.Milliseconds(),
			"remote", r.RemoteAddr,
		)
	})
}
