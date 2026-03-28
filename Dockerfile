FROM golang:1.26-alpine AS builder

# Set the Current Working Directory inside the container
WORKDIR /app

# Copy go mod and sum files first to leverage Docker cache
COPY go.mod go.sum ./

# Download all dependencies
RUN go mod download

# Copy the source code from the root of the project to the Working Directory
COPY cmd/ cmd/
COPY internal/ internal/

# Build the Go app. 
# CGO_ENABLED=0 ensures a statically linked binary which is required for Alpine
RUN CGO_ENABLED=0 GOOS=linux go build -o gateway ./cmd/gateway/main.go

# --- Stage 2: Final minimal image ---
# Start a new stage from scratch using a tiny Alpine image
FROM alpine:latest

# Add root certificates for outbound HTTPS calls (e.g., if we hit real OpenAI APIs later)
RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the Pre-built binary file from the previous stage
COPY --from=builder /app/gateway .

# Expose port 8080 to the outside world
EXPOSE 8080

ENTRYPOINT ["./gateway"]