package redis

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

type RateLimiter struct {
	client *goredis.Client
}

type RateLimitResult struct {
	Allowed    bool
	Limit      int
	Remaining  int
	RetryAfter int
	ResetAfter int
}

func NewRateLimiter(client *goredis.Client) *RateLimiter {
	return &RateLimiter{client: client}
}

// Allow uses a fixed-window counter.
// This is simple, predictable, and easy to scale horizontally.
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int, window time.Duration) (*RateLimitResult, error) {
	if limit <= 0 {
		return nil, fmt.Errorf("limit must be greater than zero")
	}
	if window <= 0 {
		return nil, fmt.Errorf("window must be greater than zero")
	}

	now := time.Now().UTC()
	windowSeconds := int(window.Seconds())
	windowStartUnix := now.Unix() - (now.Unix() % int64(windowSeconds))
	windowStart := time.Unix(windowStartUnix, 0).UTC()

	redisKey := fmt.Sprintf("%s:%s", key, windowStart.Format("20060102150405"))

	count, err := r.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return nil, err
	}

	if count == 1 {
		if err := r.client.Expire(ctx, redisKey, window).Err(); err != nil {
			return nil, err
		}
	}

	resetAfter := windowSeconds - int(now.Unix()%int64(windowSeconds))
	if resetAfter <= 0 {
		resetAfter = 1
	}

	remaining := limit - int(count)
	if remaining < 0 {
		remaining = 0
	}

	allowed := int(count) <= limit
	retryAfter := 0
	if !allowed {
		retryAfter = resetAfter
	}

	return &RateLimitResult{
		Allowed:    allowed,
		Limit:      limit,
		Remaining:  remaining,
		RetryAfter: retryAfter,
		ResetAfter: resetAfter,
	}, nil
}
