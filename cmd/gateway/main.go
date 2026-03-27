package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/powoftech/ai-inference-gateway/internal/middleware"
	"github.com/powoftech/ai-inference-gateway/internal/routing"
	pb "github.com/powoftech/ai-inference-gateway/proto/inference/v2"
)

type Gateway struct {
	lb *routing.LoadBalancer
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

	// Set necessary headers for Server-Sent Events (SSE)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Ensure the connection supports flushing
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Select the best backend using Load Balancer
	backend, err := g.lb.SelectWeightedBackend()
	if err != nil {
		http.Error(w, "Service Unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	log.Printf("[Tenant: %s] Routing to Backend: %s (Estimated Tokens: %d)", tenantID, backend.ID, tokenCount)

	// Generate a mock Request ID (in the real app, we'll use a middleware UUID)
	reqID := fmt.Sprintf("req-%d", time.Now().UnixNano())

	grpcReq := &pb.InferRequest{
		RequestId: reqID,
		ModelId:   reqBody.ModelID,
		Prompt:    reqBody.Prompt,
	}

	grpcClient := pb.NewInferenceServiceClient(backend.Client)

	start := time.Now()
	stream, err := grpcClient.InferStream(r.Context(), grpcReq)
	if err != nil {
		// CIRCUIT BREAKER: RECORD FAILURE
		log.Printf("Backend %s failed: %v", backend.ID, err)
		backend.RecordFailure()
		http.Error(w, "Backend stream failed", http.StatusBadGateway)
		return
	}

	// Read from gRPC stream and write to HTTP SSE stream
	for {
		chunk, err := stream.Recv()
		if err == io.EOF {
			break // Backend stream is finished
		}
		if err != nil {
			log.Printf("Backend %s read error: %v", backend.ID, err)
			backend.RecordFailure()
			return
		}

		// Serialize the chunk to JSON
		jsonChunk, err := json.Marshal(chunk)
		if err != nil {
			log.Printf("JSON marshaling error: %v", err)
			backend.RecordFailure()
			return
		}
		// Write in SSE format: data: {...}\n\n
		fmt.Fprintf(w, "data: %s\n\n", jsonChunk)
		flusher.Flush()
	}

	duration := time.Since(start)
	backend.RecordSuccess()
	backend.UpdateLatency(duration)

	log.Printf("Finished request on %s in %v. New EWMA: %.2fms", backend.ID, duration, backend.GetWeight())
}

func main() {
	middleware.InitTokenizer()
	middleware.InitRedisSentinel()

	backendAddrsStr := os.Getenv("BACKEND_ADDRS")
	if backendAddrsStr == "" {
		backendAddrsStr = "localhost:9091"
	}

	lb := routing.NewLoadBalancer()

	// Parse comma-separated backends and add them to the Load Balancer
	addrs := strings.Split(backendAddrsStr, ",")
	for i, addr := range addrs {
		conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		if err != nil {
			log.Fatalf("Failed to connect to backend %s: %v", addr, err)
		}
		backendID := fmt.Sprintf("backend-%d", i+1)
		lb.AddBackend(routing.NewBackend(backendID, addr, conn))
		log.Printf("Registered %s at %s (with active health checking)", backendID, addr)
	}

	gateway := &Gateway{lb: lb}
	handler := middleware.AuthMiddleware(
		middleware.TokenEstimatorMiddleware(
			middleware.RateLimitMiddleware(gateway.handleStream),
		),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/infer/stream", handler)

	port := ":8080"
	log.Printf("Gateway listening on %s", port)
	if err := http.ListenAndServe(port, mux); err != nil {
		log.Fatalf("Failed to serve gateway: %v", err)
	}
}
