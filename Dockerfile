
# The build stage
FROM golang:1.24 AS builder
WORKDIR /app

# Install CA certificates
RUN apt-get update && apt-get install -y ca-certificates && rm -rf /var/lib/apt/lists/*

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o api ./cmd/api

# The run stage
FROM scratch
WORKDIR /app

# Copy CA certificates
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# Copy the .env file into the container
COPY .env .
COPY --from=builder /app/api .

EXPOSE 8080
CMD ["./api"]
