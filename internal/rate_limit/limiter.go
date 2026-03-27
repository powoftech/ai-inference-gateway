package rate_limit

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrQuotaExceeded is returned when a tenant has no tokens left
var ErrQuotaExceeded = errors.New("quota exceeded")

// TenantQuota holds the in-memory state for a specific tenant.
// We use atomic counters or sync.Mutex for ultra-fast local deductions.
type TenantQuota struct {
	mu              sync.Mutex
	TokensRemaining int
	// In a real system, we track when the quota resets (e.g., next minute)
	ResetAt time.Time
}

// TwoTierLimiter manages the local in-memory cache and syncs to Redis
type TwoTierLimiter struct {
	// Tier 1: Local Memory. sync.Map provides highly concurrent lock-free reads
	localCache sync.Map

	// Tier 2: Redis Client would go here (omitted for local testing setup)
	// redisClient *redis.Client
}

func NewTwoTierLimiter() *TwoTierLimiter {
	limiter := &TwoTierLimiter{}

	// Seed our mock tenant for Phase 3 testing
	limiter.localCache.Store("demo-tenant", &TenantQuota{
		TokensRemaining: 500, // They get 500 tokens total
		ResetAt:         time.Now().Add(1 * time.Hour),
	})

	// In production, start a background goroutine here to sync to Redis periodically
	// go limiter.asyncRedisSync()

	return limiter
}

// Deduct verifies and deducts tokens from Tier 1 memory in < 1ms.
func (l *TwoTierLimiter) Deduct(ctx context.Context, tenantID string, tokens int) error {
	val, ok := l.localCache.Load(tenantID)
	if !ok {
		// If not in local cache, we would normally fetch from Redis here.
		// For now, fail closed.
		return errors.New("tenant not found")
	}

	quota := val.(*TenantQuota)

	quota.mu.Lock()
	defer quota.mu.Unlock()

	if quota.TokensRemaining < tokens {
		return ErrQuotaExceeded
	}

	quota.TokensRemaining -= tokens
	return nil
}

// EstimateTokens is a highly simplified token estimator.
// In Milestone 6, you will replace this with the 'tiktoken-go' library.
func EstimateTokens(prompt string) int {
	// Rough heuristic: 1 word ≈ 1.3 tokens
	words := len(prompt) / 5
	if words == 0 {
		return 1
	}
	return words
}
