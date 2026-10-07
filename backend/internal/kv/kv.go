// Package kv owns the Redis client.
package kv

import (
	"context"
	"errors"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// Open connects to Redis and verifies the connection with a ping, so a bad
// REDIS_URL fails at startup instead of on the first request.
func Open(ctx context.Context, url string) (*redis.Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	client := redis.NewClient(opts)
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, errors.Join(fmt.Errorf("ping redis: %w", err), client.Close())
	}
	return client, nil
}
