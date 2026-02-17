package middleware

import (
	"net/http"
	"time"

	"github.com/go-chi/httprate"
)

// RateLimit returns an HTTP middleware that limits requests per IP address
// to the specified number per minute. Uses a sliding window algorithm.
func RateLimit(requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.LimitByIP(requestsPerMinute, time.Minute)
}

// RateLimitByHeader returns an HTTP middleware that limits requests by
// a specific header value (e.g., X-API-Key) to the specified number per
// minute. Useful for per-key rate limiting.
func RateLimitByHeader(headerName string, requestsPerMinute int) func(http.Handler) http.Handler {
	return httprate.Limit(
		requestsPerMinute,
		time.Minute,
		httprate.WithKeyFuncs(func(r *http.Request) (string, error) {
			return r.Header.Get(headerName), nil
		}),
	)
}
