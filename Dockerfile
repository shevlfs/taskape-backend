FROM golang:1.23-alpine

WORKDIR /app

# Install protoc and essential build tools
RUN apk add --no-cache protoc protobuf-dev

# Copy everything needed for building
COPY go.mod go.sum ./
COPY taskape-proto/ ./taskape-proto/
COPY main.go ./
COPY hello.proto ./

# Download dependencies
RUN go mod download

# Build the application
RUN go build -o server main.go

# Expose gRPC port
EXPOSE 50051

CMD ["/app/server"]