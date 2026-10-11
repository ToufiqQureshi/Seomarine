package emailauth

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter counts requests per key in a fixed window.
type Limiter interface {
	// Allow records one request and reports whether it is within limit. When it
	// is not, retryAfter says how long until the window resets.
	Allow(ctx context.Context, key string, limit int, window time.Duration) (ok bool, retryAfter time.Duration, err error)
}

// RedisLimiter is a Limiter on Redis, shared across server instances.
type RedisLimiter struct{ Client *redis.Client }

// Allow increments the window counter and sets its expiry on first use.
func (l RedisLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	bucket := time.Now().Unix() / int64(window.Seconds())
	redisKey := fmt.Sprintf("emailauth:rl:%s:%d", key, bucket)
	pipe := l.Client.Pipeline()
	count := pipe.Incr(ctx, redisKey)
	pipe.Expire(ctx, redisKey, 2*window)
	if _, err := pipe.Exec(ctx); err != nil {
		return true, 0, fmt.Errorf("count auth request: %w", err)
	}
	if count.Val() > int64(limit) {
		next := time.Unix((bucket+1)*int64(window.Seconds()), 0)
		return false, time.Until(next), nil
	}
	return true, 0, nil
}
