package rate_limit

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/pkoukk/tiktoken-go"
	"github.com/redis/go-redis/v9"
)

// ErrQuotaExceeded is returned when a tenant has no tokens left
var ErrQuotaExceeded = errors.New("quota exceeded")

// TenantQuota holds the in-memory state for a specific tenant.
// We use atomic counters or sync.Mutex for ultra-fast local deductions.
type TenantQuota struct {
	mu              sync.Mutex
	TokensRemaining int
	// Track tokens consumed locally since the last Redis sync
	pendingSync int
	// In a real system, we track when the quota resets (e.g., next minute)
	ResetAt time.Time
}

// TwoTierLimiter manages the local in-memory cache and syncs to Redis
type TwoTierLimiter struct {
	// Tier 1: Local Memory. sync.Map provides highly concurrent lock-free reads
	localCache sync.Map

	// Tier 2: Redis Client for eventual consistency across distributed pods
	redisClient *redis.Client
}

func NewTwoTierLimiter(redisAddr string) *TwoTierLimiter {
	rdb := redis.NewClient(&redis.Options{
		Addr: redisAddr,
	})

	limiter := &TwoTierLimiter{
		redisClient: rdb,
	}

	// Seed our mock tenant for Phase 3 testing
	limiter.localCache.Store("demo-tenant", &TenantQuota{
		TokensRemaining: 500, // They get 500 tokens total
		ResetAt:         time.Now().Add(1 * time.Hour),
	})

	// Start the background synchronization goroutine
	go limiter.asyncRedisSync()

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
	quota.pendingSync += tokens // Track for the async worker
	return nil
}

// EstimateTokens uses standard BPE tokenization instead of heuristics.
// In Phase 3, we replace the dummy logic with 'tiktoken-go'.
func EstimateTokens(prompt string) int {
	// Note: For optimal performance in production, 'tkm' should be instantiated
	// once globally or pooled, rather than per-request. We scope it here for this milestone.
	tkm, err := tiktoken.GetEncoding("o200k_base")
	if err != nil {
		log.Printf("Tokenizer err: %v, falling back to heuristic", err)
		words := len(prompt) / 5
		if words == 0 {
			return 1
		}
		return words
	}

	token := tkm.Encode(prompt, nil, nil)
	return len(token)
}

// asyncRedisSync runs in the background, flushing local consumption to the global Redis cluster.
func (l *TwoTierLimiter) asyncRedisSync() {
	// As per your Technical Spec, this is the "Eventual Consistency" window
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	ctx := context.Background()

	for range ticker.C {
		l.localCache.Range(func(key, value interface{}) bool {
			tenantID := key.(string)
			quota := value.(*TenantQuota)

			quota.mu.Lock()
			consumed := quota.pendingSync
			quota.pendingSync = 0 // Reset local counter
			quota.mu.Unlock()

			if consumed > 0 {
				// Decrement global Redis state
				err := l.redisClient.DecrBy(ctx, "quota:"+tenantID, int64(consumed)).Err()
				if err != nil {
					log.Printf("Failed to sync tenant %s to Redis: %v", tenantID, err)
					// Revert the pending tokens so we try again next tick
					quota.mu.Lock()
					quota.pendingSync += consumed
					quota.mu.Unlock()
				} else {
					log.Printf("Background Sync: Deducted %d tokens from Redis for %s", consumed, tenantID)
				}
			}
			return true
		})
	}
}
