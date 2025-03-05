FROM golang:1.24-alpine AS builder

WORKDIR /app

# Copy go mod and sum files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN go build -o taskape-backend .

FROM alpine:latest

WORKDIR /app

# Copy the binary from builder
COPY --from=builder /app/taskape-backend .
# Copy .env file for configuration
COPY .env ./

# Expose port
EXPOSE 50051

# Command to run
CMD ["./taskape-backend"]