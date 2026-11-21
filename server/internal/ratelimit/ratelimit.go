// Package ratelimit is a per-learner quota for expensive (AI) routes, kept in Redis so
// it survives restarts and holds across instances.
package ratelimit

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/shaunmortal/AI-tutor/server/internal/auth"
	"github.com/shaunmortal/AI-tutor/server/internal/httpx"
)

// Limit allows max requests per window per learner, keyed on the verified user id —
// never on an IP or a header the caller controls. Must sit behind auth.Require.
func Limit(rdb *redis.Client, name string, max int64, window time.Duration, message string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := keyFor(name, auth.UserID(r))
			ctx := r.Context()

			n, err := hit(ctx, rdb, key, window)
			if err != nil {
				// Fail open: a Redis outage should slow nobody's studying.
				slog.Warn("rate limit unavailable", "err", err)
				next.ServeHTTP(w, r)
				return
			}
			if n > max {
				ttl, _ := rdb.TTL(ctx, key).Result()
				httpx.Fail(w, http.StatusTooManyRequests, "RATE_LIMITED",
					fmt.Sprintf("%s Try again in %d seconds.", message, int(ttl.Seconds())))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Allow is the same quota for paid work inside a handler that has a fallback instead of a 429:
// it counts one use and says whether the learner is still within max per window. Fails open on
// a Redis error, as Limit does.
func Allow(ctx context.Context, rdb *redis.Client, name, userID string, max int64, window time.Duration) bool {
	n, err := hit(ctx, rdb, keyFor(name, userID), window)
	if err != nil {
		slog.Warn("rate limit unavailable", "err", err)
		return true
	}
	return n <= max
}

func keyFor(name, userID string) string { return fmt.Sprintf("rl:%s:%s", name, userID) }

// hit counts one use in a fixed window: INCR, and start the clock on the first hit.
func hit(ctx context.Context, rdb *redis.Client, key string, window time.Duration) (int64, error) {
	n, err := rdb.Incr(ctx, key).Result()
	if err == nil && n == 1 {
		rdb.Expire(ctx, key, window)
	}
	return n, err
}
