package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	"github.com/pkoukk/tiktoken-go"
)

const TokenCountKey ContextKey = "token_count"

// Tokenizer Singleton
var tke *tiktoken.Tiktoken

// Initialize it once at startup
func InitTokenizer() {
	var err error
	// cl100k_base is the standard encoding for OpenAI models, often used as a baseline for LLama/Mistral estimation
	tke, err = tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		log.Fatalf("Failed to initialize tiktoken: %v", err)
	}
	log.Println("Tiktoken encoder initialized successfully")
}

// TokenEstimatorMiddleware reads the prompt, estimates tokens, and restores the request body
func TokenEstimatorMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// 1. Read the body
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Failed to read request body", http.StatusInternalServerError)
			return
		}

		// 2. Parse exactly what we need (the prompt)
		var payload struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(bodyBytes, &payload); err != nil {
			http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
			return
		}

		// 3. Count the tokens
		tokenCount := len(tke.Encode(payload.Prompt, nil, nil))

		// Log the profiling (Important for your G-01 Goal)
		log.Printf("Token estimation took %v for %d tokens", time.Since(start), tokenCount)

		// 4. RESTORE the body so the next handler can read it
		r.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

		// 5. Inject Token Count into context
		ctx := context.WithValue(r.Context(), TokenCountKey, tokenCount)

		next.ServeHTTP(w, r.WithContext(ctx))
	}
}
