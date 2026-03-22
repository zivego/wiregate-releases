package api

import (
	"context"
	"log"
	"time"

	"github.com/zivego/wiregate/internal/persistence/ratelimitrepo"
)

// dbRateLimiter is a DB-backed sliding window rate limiter that persists across
// restarts and works across multiple server instances.
type dbRateLimiter struct {
	repo   *ratelimitrepo.Repo
	maxN   int
	window time.Duration
}

// newDBRateLimiter creates a rate limiter backed by the rate_limit_entries table.
func newDBRateLimiter(repo *ratelimitrepo.Repo, maxN int, window time.Duration) rateLimiter {
	return &dbRateLimiter{
		repo:   repo,
		maxN:   maxN,
		window: window,
	}
}

func (rl *dbRateLimiter) allow(key string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	now := time.Now().UTC()
	cutoff := now.Add(-rl.window)

	count, err := rl.repo.Count(ctx, key, cutoff)
	if err != nil {
		// On DB error, fail open (allow the request) to avoid blocking users.
		return true
	}
	if count >= rl.maxN {
		return false
	}

	if err := rl.repo.Record(ctx, key, now); err != nil {
		// Record failed but count was under limit — allow.
		return true
	}
	return true
}

// StartRateLimitCleanup runs a background goroutine that periodically removes
// expired rate limit entries. It stops when the context is cancelled.
func StartRateLimitCleanup(ctx context.Context, repo *ratelimitrepo.Repo, interval time.Duration, logger *log.Logger) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				cutoff := time.Now().UTC().Add(-5 * time.Minute)
				deleted, err := repo.Cleanup(ctx, cutoff)
				if err != nil {
					logger.Printf("rate limit cleanup error: %v", err)
				} else if deleted > 0 {
					logger.Printf("rate limit cleanup: removed %d expired entries", deleted)
				}
			}
		}
	}()
}
