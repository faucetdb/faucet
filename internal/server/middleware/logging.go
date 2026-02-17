package middleware

import (
	"log/slog"
	"net/http"
	"time"
)

// Logger returns an HTTP middleware that logs every request using structured
// logging. It captures the method, path, status code, response size, duration,
// request ID, and remote address.
func Logger(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := &responseWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(ww, r)

			duration := time.Since(start)
			level := slog.LevelInfo
			if ww.status >= 500 {
				level = slog.LevelError
			} else if ww.status >= 400 {
				level = slog.LevelWarn
			}

			logger.Log(r.Context(), level, "request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", ww.status,
				"duration_ms", float64(duration.Microseconds())/1000.0,
				"bytes", ww.bytes,
				"request_id", GetRequestID(r.Context()),
				"remote_addr", r.RemoteAddr,
			)
		})
	}
}

// responseWriter wraps http.ResponseWriter to capture the status code and
// bytes written for logging purposes.
type responseWriter struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func (w *responseWriter) WriteHeader(code int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *responseWriter) Write(b []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += n
	return n, err
}

// Unwrap returns the underlying ResponseWriter, required for http.Flusher
// and other interface assertions through middleware chains.
func (w *responseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
