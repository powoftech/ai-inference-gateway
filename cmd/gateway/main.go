package main

import (
	"log"
	"net/http"
	"os"

	pb "github.com/powoftech/ai-inference-gateway/internal/gen/inference/v1"
	"github.com/powoftech/ai-inference-gateway/internal/handler"
	"github.com/powoftech/ai-inference-gateway/internal/rate_limit"
	"github.com/powoftech/ai-inference-gateway/internal/telemetry"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// PHASE 4 UPGRADE: 12-Factor App Configuration
	// Pull routing addresses from the environment for Kubernetes compatibility.
	backendAddr := os.Getenv("BACKEND_ADDR")
	if backendAddr == "" {
		backendAddr = "dns:///backend.default.svc.cluster.local:50051"
	}

	redisAddr := os.Getenv("REDIS_ADDR")
	if redisAddr == "" {
		redisAddr = "localhost:6379"
	}

	conn, err := grpc.NewClient(
		backendAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Enable default RoundRobin load balancing across resolved IP addresses.
		// In Phase 6 (Tuning), you will replace this with your custom EWMA Balancer.
		grpc.WithDefaultServiceConfig(`{"loadBalancingPolicy":"round_robin"}`),
	)
	if err != nil {
		log.Fatalf("Failed to connect to backend: %v", err)
	}
	defer conn.Close()

	modelClient := pb.NewModelServiceClient(conn)

	// Initialize the new Rate Limiter with a dynamic Redis connection
	limiter := rate_limit.NewTwoTierLimiter(redisAddr)

	// Initialize Telemetry
	telemetry.InitMetrics()

	// Inject the limiter into the handler
	chatHandler := handler.NewChatHandler(modelClient, limiter)

	http.HandleFunc("/v1/chat/completions", chatHandler.HandleChatCompletions)

	// Expose Prometheus metrics endpoint
	http.Handle("/metrics", telemetry.MetricsHandler())

	port := ":8080"
	log.Printf("Go Gateway (Rate Limited) listening on %s...", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
