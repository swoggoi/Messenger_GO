# ============ STAGE 1: Build ============
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download

COPY . .


RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-w -s" -o /messenger ./cmd/messenger

FROM alpine:latest


RUN apk add --no-cache ca-certificates tzdata curl

WORKDIR /app
COPY --from=builder /messenger /app/messenger
COPY --from=builder /app/web /app/web
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8080/api/v1/auth/login || exit 1

CMD ["/app/messenger"]