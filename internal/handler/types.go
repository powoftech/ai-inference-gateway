package handler

// ChatCompletionRequest represents the standard OpenAI API payload.
// We map this strictly to ensure compatibility with standard AI tooling.
type ChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"` // Mandated to true for this streaming gateway
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
