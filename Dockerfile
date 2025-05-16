# Build stage
FROM golang:1.24.3-alpine AS builder

# Install necessary build tools
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum* ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -o /blockchain-gateway ./cmd/server

# Development stage (for development with Air hot-reload)
FROM golang:1.24.3-alpine AS development

# Install Air for live reload and necessary tools
RUN go install github.com/cosmtrek/air@latest && \
    apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy the entire source code
COPY . .

# Download dependencies
RUN go mod download

# Expose port
EXPOSE 8080

# Set environment to development
ENV GIN_MODE=debug

# Command to run Air for hot-reloading
CMD ["air", "-c", ".air.toml"]

# Final stage (for production)
FROM alpine:3.18 AS production

# Add CA certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /blockchain-gateway .

# Copy config files if needed
COPY .env* ./

# Create a non-root user and switch to it
RUN adduser -D -g '' appuser && \
    chown -R appuser:appuser /app
USER appuser

# Expose port
EXPOSE 8080

# Set environment variables
ENV GIN_MODE=release

# Command to run
CMD ["./blockchain-gateway"]