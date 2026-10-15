FROM golang:1.24.0-alpine3.21 AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN go build -ldflags "-s -w" -o ./bin/ .

FROM alpine:3.21

RUN apk add --no-cache tzdata \
    && addgroup -S app \
    && adduser -S -u 1000 -G app app \
    && mkdir -p /app \
    && chown -R app:app /app

COPY --from=builder /src/bin /app/rss-reader

WORKDIR /app

EXPOSE 8080

ENV TZ=Asia/Shanghai
USER app

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD wget -q -O /dev/null http://127.0.0.1:8080/health || exit 1

ENTRYPOINT ["/app/rss-reader"]
