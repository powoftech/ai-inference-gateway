package middleware

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

var redisClient *redis.Client

// rateLimitScript is the Lua script from our TDD that deducts based on requested tokens
const rateLimitScript = `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local requested_tokens = tonumber(ARGV[4])

local bucket = redis.call('HMGET', key, 'tokens', 'last_refill')
local tokens = tonumber(bucket[1]) or capacity
local last = tonumber(bucket[2]) or now

local elapsed = math.max(0, now - last)
tokens = math.min(capacity, tokens + (elapsed * rate / 1000))

if tokens < requested_tokens then
    return { 0, math.ceil(((requested_tokens - tokens) / rate) * 1000) }
end

tokens = tokens - requested_tokens
redis.call('HMSET', key, 'tokens', tokens, 'last_refill', now)
redis.call('PEXPIRE', key, math.ceil((capacity / rate) * 1000) * 2)

return { 1, 0 }
`

func InitRedisSentinel() {
	// Read Sentinel Address from ENV, fallback to localhost
	sentinelAddr := os.Getenv("REDIS_SENTINEL_ADDR")
	if sentinelAddr == "" {
		sentinelAddr = "localhost:26379"
	}

	// Connect using Sentinel to ensure High Availability
	redisClient = redis.NewFailoverClient(&redis.FailoverOptions{
		MasterName:    "mymaster",
		SentinelAddrs: []string{sentinelAddr},
		Password:      "", // No password for local dev

	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Printf("Warning: Failed to connect to Redis Sentinel at %s: %v. Rate limiter will fail open.", sentinelAddr, err)
	} else {
		log.Printf("Connected to Redis Sentinel successfully at %s", sentinelAddr)
	}
}

func RateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tenantID, ok := r.Context().Value(TenantIDKey).(string)
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		tokenCount, ok := r.Context().Value(TokenCountKey).(int)
		if !ok {
			// Fallback if token estimation failed
			tokenCount = 1
		}

		// Configure quota (In production, load this from DB per tenant)
		// For now, let's say tenants get 5000 tokens per second, max burst of 10000
		rate := 5000
		capacity := 10000
		now := time.Now().UnixMilli()
		key := "tenant:" + tenantID + ":rl"

		// Execute the Lua script atomically
		result, err := redisClient.Eval(r.Context(), rateLimitScript, []string{key}, rate, capacity, now, tokenCount).Result()

		if err != nil {
			log.Printf("Redis error (failing open): %v", err)
			// If Redis is down, we "fail open" (allow the request) and log it, or apply an in-memory fallback.
			// For this iteration, failing open ensures uptime.
			next.ServeHTTP(w, r)
			return
		}

		resArr := result.([]interface{})
		allowed := resArr[0].(int64) == 1
		retryAfterMs := resArr[1].(int64)

		if !allowed {
			w.Header().Set("Retry-After", strconv.FormatInt(retryAfterMs/1000, 10))
			http.Error(w, "Rate limit exceeded. Not enough token quota.", http.StatusTooManyRequests)
			log.Printf("[Tenant: %s] Rate limited. Needed %d tokens.", tenantID, tokenCount)
			return
		}

		next.ServeHTTP(w, r)
	}
}
