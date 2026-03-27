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
	"github.com/powoftech/ai-inference-gateway/internal/rate_limit"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type ChatHandler struct {
	ModelClient pb.ModelServiceClient
	Limiter     *rate_limit.TwoTierLimiter // Inject Limiter
}

func NewChatHandler(client pb.ModelServiceClient, limiter *rate_limit.TwoTierLimiter) *ChatHandler {
	return &ChatHandler{
		ModelClient: client,
		Limiter:     limiter,
	}
}

func (h *ChatHandler) HandleChatCompletions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Bad request: invalid payload", http.StatusBadRequest)
		return
	}

	// --- NEW: RATE LIMITING PHASE ---
	tenantID := "demo-tenant" // Typically extracted from Authorization Header

	// 1. Estimate prompt tokens
	promptContent := ""
	if len(req.Messages) > 0 {
		promptContent = req.Messages[0].Content
	}
	estimatedTokens := rate_limit.EstimateTokens(promptContent)

	// 2. Add an arbitrary buffer for the generated output (e.g., we assume they will generate at least 20 tokens)
	totalRequestedTokens := estimatedTokens + 20

	// 3. Deduct from local Tier 1 memory. This takes < 1ms.
	if err := h.Limiter.Deduct(r.Context(), tenantID, totalRequestedTokens); err != nil {
		if errors.Is(err, rate_limit.ErrQuotaExceeded) {
			log.Printf("Tenant %s rate limited. Tokens requested: %d", tenantID, totalRequestedTokens)
			http.Error(w, "429 Too Many Requests: Token Quota Exceeded", http.StatusTooManyRequests)
			return
		}
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	// --------------------------------

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

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
		TenantId:  tenantID,
	}

	stream, err := h.ModelClient.GenerateStream(r.Context(), grpcReq)
	if err != nil {
		http.Error(w, "Backend unavailable", http.StatusServiceUnavailable)
		return
	}

	for {
		resp, err := stream.Recv()

		if errors.Is(err, io.EOF) {
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return
		}

		if err != nil {
			if status.Code(err) == codes.Canceled {
				return
			}
			return
		}

		chunk := ChatCompletionChunk{
			ID:      resp.RequestId,
			Object:  "chat.completion.chunk",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []Choice{
				{Index: 0, Delta: Delta{Content: resp.Token}},
			},
		}

		if resp.IsFinished {
			finishReason := "stop"
			chunk.Choices[0].FinishReason = &finishReason
		}

		chunkBytes, _ := json.Marshal(chunk)
		fmt.Fprintf(w, "data: %s\n\n", string(chunkBytes))
		flusher.Flush()
	}
}
