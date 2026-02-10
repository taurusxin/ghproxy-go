# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# CGO_ENABLED=0 for static binary
RUN CGO_ENABLED=0 GOOS=linux go build -o ghproxy-go .

# Final stage
FROM alpine:latest

# Install CA certificates for HTTPS
RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/ghproxy-go .

# Environment variables
ENV HOST=0.0.0.0
ENV PORT=8972
ENV GIN_MODE=release

EXPOSE 8972

CMD ["./ghproxy-go"]
