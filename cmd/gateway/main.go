package main

import (
	"log"
	"net/http"

	pb "github.com/powoftech/ai-inference-gateway/internal/gen/inference/v1"
	"github.com/powoftech/ai-inference-gateway/internal/handler"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	// 1. Establish connection to the Mock Python Backend
	// In production, this address will be resolved via Kubernetes DNS or our EWMA router.
	backendAddr := "localhost:50051"

	conn, err := grpc.NewClient(backendAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Failed to connect to backend: %v", err)
	}
	defer conn.Close()

	// 2. Initialize the gRPC client
	modelClient := pb.NewModelServiceClient(conn)

	// 3. Inject the client into our HTTP handler
	chatHandler := handler.NewChatHandler(modelClient)

	// 4. Register the route
	http.HandleFunc("/v1/chat/completions", chatHandler.HandleChatCompletions)

	port := ":8080"
	log.Printf("Go Gateway Multiplexer listening on %s...", port)

	if err := http.ListenAndServe(port, nil); err != nil {
		log.Fatalf("Failed to start HTTP server: %v", err)
	}
}
