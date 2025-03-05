# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the backend code
COPY . .

# Build the backend
RUN go build -mod=vendor -o taskape-backend .

# --- Final stage ---
FROM alpine:latest

WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder /app/taskape-backend .

# Expose port
EXPOSE 50051

# Run the backend
CMD ["./taskape-backend"]
