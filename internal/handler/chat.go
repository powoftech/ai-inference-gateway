package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"time"

	pb "github.com/powoftech/ai-inference-gateway/internal/gen/inference/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	ModelClient pb.ModelServiceClient
}

func NewChatHandler(client pb.ModelServiceClient) *ChatHandler {
	return &ChatHandler{ModelClient: client}
}

func (h *ChatHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request: invalid JSON payload", http.StatusBadRequest)
		return
	}

	// 1. Setup Server-Sent Events (SSE) Headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	// Allow CORS for easy testing from frontend clients
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// 2. Map HTTP request to gRPC contract
	var grpcMessages []*pb.Message
	for _, m := range req.Messages {
		grpcMessages = append(grpcMessages, &pb.Message{
			Role:    m.Role,
			Content: m.Content,
		})
	}

	grpcReq := &pb.GenerateRequest{
		RequestId: fmt.Sprintf("req-%d", time.Now().UnixNano()),
		Model:     req.Model,
		Messages:  grpcMessages,
		TenantId:  "demo-tenant", // Hardcoded for Phase 2; Phase 3 will pull from API keys
	}

	// 3. Initiate gRPC Stream with Context Propagation (Milestone 5)
	// Notice we pass `r.Context()`. If the HTTP client drops, this context is canceled,
	// immediately closing the gRPC stream to the backend.
	stream, err := h.ModelClient.GenerateStream(r.Context(), grpcReq)
	if err != nil {
		log.Printf("Failed to call backend: %v", err)
		http.Error(w, "Backend unavailable", http.StatusServiceUnavailable)
		return
	}

	// 4. Multiplex gRPC stream to HTTP SSE
	for {
		resp, err := stream.Recv()

		if errors.Is(err, io.EOF) {
			// Backend finished sending tokens
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		if err != nil {
			// Handle client disconnects gracefully without throwing massive errors
			if status.Code(err) == codes.Canceled {
				log.Printf("Client disconnected, canceling backend generation for request: %s", grpcReq.RequestId)
				return
			}
			log.Printf("Error receiving from stream: %v", err)
			return
		}

		// Map backend response to OpenAI format
		chunk := ChatCompletionChunk{
			ID:      resp.RequestId,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []Choice{
				{
					Index: 0,
					Delta: Delta{
						Content: resp.Token,
					},
				},
			},
		}

		if resp.IsFinished {
			finishReason := "stop"
			chunk.Choices[0].FinishReason = &finishReason
		}

		chunkBytes, _ := json.Marshal(chunk)

		// Write standard SSE payload
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		flusher.Flush() // Force the buffer to flush down to the client immediately
	}
}
