package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	pb "github.com/powoftech/ai-inference-gateway/proto/inference/v2"
)

type mockBackendServer struct {
	pb.UnimplementedInferenceServiceServer
}

// InferStream simulates a streaming LLM by yielding words one by one
func (s *mockBackendServer) InferStream(req *pb.InferRequest, stream pb.InferenceService_InferStreamServer) error {
	log.Printf("Received streaming request: %s for model: %s", req.RequestId, req.ModelId)

	// Simulate processing time (Time-To-First-Token)
	time.Sleep(100 * time.Millisecond)

	// A mock response to stream back
	mockResponse := fmt.Sprintf("Hello! This is a simulated streaming response for your prompt: '%s'", req.Prompt)
	tokens := strings.Split(mockResponse, " ")

	for i, token := range tokens {
		// Simulate token generation time
		time.Sleep(50 * time.Millisecond)

		chunk := &pb.InferChunk{
			RequestId: req.RequestId,
			Text:      token + " ", // Add space back for formatting
			IsFinal:   i == len(tokens)-1,
		}

		if chunk.IsFinal {
			chunk.GeneratedTokens = int64(len(tokens))
		}

		if err := stream.Send(chunk); err != nil {
			log.Printf("Error sending chunk: %v", err)
			return err
		}
	}

	log.Printf("Finished streaming request: %s", req.RequestId)
	return nil
}

func (s *mockBackendServer) HealthCheck(ctx context.Context, req *pb.HealthRequest) (*pb.HealthResponse, error) {
	return &pb.HealthResponse{Status: "SERVING"}, nil
}

func main() {
	port := ":9091"
	lis, err := net.Listen("tcp", port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterInferenceServiceServer(grpcServer, &mockBackendServer{})
	reflection.Register(grpcServer)

	log.Printf("Mock Backend gRPC server listening on %s", port)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
