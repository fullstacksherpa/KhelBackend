# Stage 1: Install dependencies
FROM golang:1.24-bookworm AS deps

WORKDIR /app

COPY go.mod go.sum ./

RUN go mod download

# Stage 2: Build the application
FROM golang:1.24-bookworm AS builder

WORKDIR /app

COPY --from=deps /go/pkg /go/pkg
COPY . .
COPY .env .


ENV CGO_ENABLED=0
ENV GOOS=linux

# Output binary as main
RUN go build -ldflags="-w -s" -o main ./cmd/api

# Final stage: Run the application
FROM debian:bookworm-slim

WORKDIR /app

# Install CA certificates (💥 this fixes TLS errors like x509 unknown authority)
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*


# Create a non-root user and group
RUN groupadd -r appuser && useradd -r -g appuser appuser

# Copy the built application
COPY --from=builder /app/main .

# Change ownership of the application binary
RUN chown appuser:appuser /app/main

# Switch to the non-root user
USER appuser

CMD ["./main"]