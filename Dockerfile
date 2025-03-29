# Build stage
FROM golang:1.23.4-alpine AS builder

WORKDIR /app

# Copy the entire repo with vendored dependencies
COPY . .

# Build using the vendor directory
RUN go build -mod=vendor -o taskape-backend .

# Final stage
FROM alpine:latest

WORKDIR /app

# Install any runtime dependencies
RUN apk add --no-cache ca-certificates

# Copy binary from the build stage
COPY --from=builder /app/taskape-backend .
COPY schema.sql /app/schema.sql

# Expose the gRPC port
EXPOSE 50051

# Run the application
CMD ["./taskape-backend"]