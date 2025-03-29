# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o taskape-backend .

# Final stage
FROM alpine:latest

WORKDIR /app

COPY --from=builder /app/taskape-backend .
COPY schema.sql /app/schema.sql

EXPOSE 50051

CMD ["./taskape-backend"]