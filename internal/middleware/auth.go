package middleware

import (
	"context"
	"net/http"
	"strings"
)

// ContextKey is a custom type to avoid context key collisions
type ContextKey string

const TenantIDKey ContextKey = "tenant_id"

// Mock tenant database (In Week 4, this will be replaced by an LRU Cache + Redis)
var mockApiKeys = map[string]string{
	"sk-green-node-123": "tenant-alpha",
	"sk-green-node-456": "tenant-beta",
}

// AuthMiddleware extracts the API key and injects the tenant ID into the context
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
			http.Error(w, "Missing or invalid Authorization header", http.StatusUnauthorized)
			return
		}

		apiKey := strings.TrimPrefix(authHeader, "Bearer ")

		tenantID, exists := mockApiKeys[apiKey]
		if !exists {
			http.Error(w, "Invalid API Key", http.StatusUnauthorized)
			return
		}

		// Inject Tenant ID into the request context
		ctx := context.WithValue(r.Context(), TenantIDKey, tenantID)

		// Pass the new context to the next handler
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
