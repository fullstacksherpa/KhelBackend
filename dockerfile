# ================================
# Stage 1: Download Go dependencies
# ================================
FROM golang:1.24-bookworm AS deps

WORKDIR /app

# Copy only go.mod and go.sum first for dependency caching
COPY go.mod go.sum ./
RUN go mod download

# ================================
# Stage 2: Build the Go binary
# ================================
FROM golang:1.24-bookworm AS builder

WORKDIR /app

# Copy cached dependencies from deps stage
COPY --from=deps /go/pkg /go/pkg

# Copy the entire project
COPY . .

# Disable CGO to build static binary
ENV CGO_ENABLED=0
ENV GOOS=linux

# Build the Go binary with optimizations
RUN go build -ldflags="-w -s" -o main ./cmd/api

# ================================
# Stage 3: Final lightweight image
# ================================
FROM gcr.io/distroless/base-debian12

WORKDIR /app

# Copy the binary and set permissions (no need to create user/group manually)
COPY --from=builder --chown=nonroot:nonroot /app/main .

# Run as non-root user for security
USER nonroot:nonroot

# Run the app
CMD ["./main"]