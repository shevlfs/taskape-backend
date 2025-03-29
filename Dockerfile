# --- Build stage ---
FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -o taskape-backend .

WORKDIR /app

COPY --from=builder /app/taskape-backend .

EXPOSE 50051

CMD ["./taskape-backend"]