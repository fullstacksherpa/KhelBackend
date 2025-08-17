# ================================
# Stage 1: Download Go dependencies
# ================================
FROM golang:1.24-bookworm AS deps

WORKDIR /app

# Copy go.mod and go.sum for caching
COPY go.mod go.sum ./
RUN go mod download

# ================================
# Stage 2: Build the Go binary
# ================================
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy cached dependencies
COPY --from=deps /go/pkg /go/pkg

# Copy the entire project
COPY . .

# Disable CGO for static binary
ENV CGO_ENABLED=0
ENV GOOS=linux

# Build the binary
RUN go build -ldflags="-w -s" -o main ./cmd/api

# ================================
# Stage 3: Final lightweight image
# ================================
FROM debian:bookworm-slim

WORKDIR /app

# Install curl for healthchecks
RUN apt-get update && apt-get install -y curl && rm -rf /var/lib/apt/lists/*

# Add a non-root user
RUN useradd -ms /bin/bash nonroot

# Copy binary from builder
COPY --from=builder /app/main .


# Switch to non-root
USER nonroot

# Run the app
CMD ["./main"]
