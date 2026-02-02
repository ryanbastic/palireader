# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY main.go ./

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o palireader .

# Runtime stage
FROM alpine:latest

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/palireader .

# Copy the Pali text data directory
COPY 2_pali/ ./2_pali/

# Expose the application port
EXPOSE 8000

# Run the application
CMD ["./palireader"]
