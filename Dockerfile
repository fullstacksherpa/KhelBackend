# The build stage
FROM golang:1.24-alpine AS builder
WORKDIR /app

# Install CA certificates and other dependencies
RUN apk add --no-cache ca-certificates

# Copy Go module files separately to leverage Docker's build cache
COPY go.mod go.sum ./
RUN go mod download

# Copy application source code
COPY . .

# Build the Go application for Linux with CGO disabled (static binary)
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/api

# The run stage
FROM alpine:latest
WORKDIR /app

# Install CA certificates (Alpine base image doesn't have them by default)
RUN apk add --no-cache ca-certificates

# Copy the compiled binary from the builder stage
COPY --from=builder /app/api .

# Expose the port that the application will use
EXPOSE 8080

# Start the application
CMD ["./api"]
