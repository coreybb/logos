# Build stage
FROM golang:1.24-bookworm AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o logos .

# Runtime stage
FROM debian:bookworm-slim

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
    ca-certificates \
    pandoc && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=builder /app/logos .

EXPOSE 8080

CMD ["./logos"]
