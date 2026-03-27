package main

import (
	"log"
	"net/http"

	pb "github.com/powoftech/ai-inference-gateway/internal/gen/inference/v1"
	"github.com/powoftech/ai-inference-gateway/internal/handler"
	"github.com/powoftech/ai-inference-gateway/internal/rate_limit"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	backendAddr := "localhost:50051"

	conn, err := grpc.NewClient(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to backend: %v", err)
	}
	defer conn.Close()

	modelClient := pb.NewModelServiceClient(conn)

	// Initialize the new Rate Limiter with a local Redis connection
	limiter := rate_limit.NewTwoTierLimiter("localhost:6379")

	// Inject the limiter into the handler
	chatHandler := handler.NewChatHandler(modelClient, limiter)

	http.HandleFunc("/v1/chat/completions", chatHandler.HandleChatCompletions)

	port := ":8080"
	log.Printf("Go Gateway (Rate Limited) listening on %s...", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
