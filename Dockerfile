# Stage 1: Build UI
FROM node:22-alpine AS ui-builder
WORKDIR /app/ui
COPY ui/package*.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Stage 2: Build Go binary
FROM golang:1.25-alpine AS go-builder
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui-builder /app/internal/ui/dist ./internal/ui/dist
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.version=$(git describe --tags --always 2>/dev/null || echo dev)" \
    -o /faucet ./cmd/faucet

# Stage 3: Final image (alpine for shell access + minimal size)
FROM alpine:3.21
RUN apk add --no-cache ca-certificates
COPY --from=go-builder /faucet /usr/local/bin/faucet

RUN addgroup -S faucet && adduser -S faucet -G faucet
RUN mkdir -p /data && chown faucet:faucet /data

ENV FAUCET_DATA_DIR=/data
EXPOSE 8080
VOLUME /data

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget --spider -q http://localhost:8080/healthz || exit 1

USER faucet
ENTRYPOINT ["faucet"]
CMD ["serve", "--foreground", "--host", "0.0.0.0"]
