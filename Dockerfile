# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go.mod and go.sum
COPY go.mod go.sum ./
RUN go mod download

# Copy the rest of the backend code
COPY . .

# Build the backend
RUN go build -o taskape-backend .

# --- Final stage ---
FROM alpine:latest

# Install PostgreSQL client for database checks
RUN apk add --no-cache postgresql-client

WORKDIR /app

# Copy the compiled binary from builder stage
COPY --from=builder /app/taskape-backend .

# Expose port
EXPOSE 50051

# Set the entrypoint script
ENTRYPOINT ["/app/entrypoint.sh"]

# Run the backend
CMD ["./taskape-backend"]