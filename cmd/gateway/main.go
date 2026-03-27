package main

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/powoftech/ai-inference-gateway/internal/handler"
)

// chatCompletionsHandler acts as our HTTP sink for Day 1 baseline testing.
// It simply validates the incoming OpenAI-formatted payload and returns 200 OK.
func chatCompletionsHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req handler.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request: invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Logging to verify standard payload parsing (Disable in production load tests)
	log.Printf("Received baseline request for model: %s with %d messages. Stream: %v\n", req.Model, len(req.Messages), req.Stream)

	// Return an immediate 200 OK to establish our absolute maximum bare-metal HTTP throughput
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "baseline_received"}`))
}

func main() {
	http.HandleFunc("/v1/chat/completions", chatCompletionsHandler)

	port := ":8080"
	log.Printf("Go Gateway Baseline (Zero-logic) listening on %s...", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
