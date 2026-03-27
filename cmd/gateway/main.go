package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/powoftech/ai-inference-gateway/internal/middleware"
	pb "github.com/powoftech/ai-inference-gateway/proto/inference/v2"
)

type Gateway struct {
	grpcClient pb.InferenceServiceClient
}

// REST Request payload
type InferRESTRequest struct {
	ModelID string `json:"model_id"`
	Prompt  string `json:"prompt"`
}

func (g *Gateway) handleStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Retrieve values injected by middlewares
	tenantID, _ := r.Context().Value(middleware.TenantIDKey).(string)
	tokenCount, _ := r.Context().Value(middleware.TokenCountKey).(int)

	var reqBody InferRESTRequest
	if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	// Generate a mock Request ID (in the real app, we'll use a middleware UUID)
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	// Set necessary headers for Server-Sent Events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Allow cross-origin for easy testing
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Ensure the connection supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Prepare gRPC Request
	grpcReq := &pb.InferRequest{
		RequestId: reqID,
		ModelId:   reqBody.ModelID,
		Prompt:    reqBody.Prompt,
	}

	log.Printf("[Tenant: %s] Forwarding request %s (Estimated Tokens: %d) to backend...", tenantID, reqID, tokenCount)

	// Call the gRPC backend
	stream, err := g.grpcClient.InferStream(r.Context(), grpcReq)
	if err != nil {
		log.Printf("Failed to call backend: %v", err)
		return
	}

	// Read from gRPC stream and write to HTTP SSE stream
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break // Backend stream is finished
		}
		if err != nil {
			log.Printf("Error reading from stream: %v", err)
			return
		}

		// Serialize the chunk to JSON
		jsonChunk, err := json.Marshal(chunk)
		if err != nil {
			log.Printf("JSON serialization error: %v", err)
			continue
		}

		// Write in SSE format: data: {...}\n\n
		fmt.Fprintf(w, "data: %s\n\n", jsonChunk)
		flusher.Flush() // Crucial: Immediately push the buffer to the client
	}

	log.Printf("Completed streaming request %s", reqID)
}

func main() {
	// 1. Initialize our singletons
	middleware.InitTokenizer()

	// 2. Initialize Redis Sentinel client for rate limiting
	middleware.InitRedisSentinel()

	// Read Backend Address from ENV, fallback to localhost for native dev
	backendAddr := os.Getenv("BACKEND_ADDR")
	if backendAddr == "" {
		backendAddr = "localhost:9091"
	}
	conn, err := grpc.NewClient(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Did not connect to backend: %v", err)
	}
	defer conn.Close()

	gateway := &Gateway{
		grpcClient: pb.NewInferenceServiceClient(conn),
	}

	// 3. Chain the middlewares: Auth -> Tokenizer -> RateLimiter -> handleStream
	handler := middleware.AuthMiddleware(
		middleware.TokenEstimatorMiddleware(
			middleware.RateLimitMiddleware(gateway.handleStream),
		),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/infer/stream", handler)

	port := ":8080"
	log.Printf("Gateway listening for HTTP/2 SSE on %s (Backend: %s)", port, backendAddr)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to serve gateway: %v", err)
	}
}
