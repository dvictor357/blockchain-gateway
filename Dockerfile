# Build stage
FROM golang:1.25.5-alpine AS builder

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
FROM golang:1.25.5-alpine AS development

# Install Air for live reload and necessary tools
RUN go install github.com/air-verse/air@v1.63.0 && \
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
ENV APP_MODE=debug

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

# Database migrations are applied at startup. Configuration (including any .env values)
# is supplied at runtime via environment variables and is never baked into the image.
COPY --from=builder /app/migrations ./migrations

# Create a non-root user and switch to it
RUN adduser -D -g '' appuser && \
    chown -R appuser:appuser /app
USER appuser

# Expose port
EXPOSE 8080

# Set environment variables
ENV APP_MODE=release

# Command to run
CMD ["./blockchain-gateway"]