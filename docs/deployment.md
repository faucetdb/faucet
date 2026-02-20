# Deployment Guide

Faucet is a single static binary with no runtime dependencies. It can be deployed as a Docker container, a systemd service, or behind a reverse proxy.

## Docker Deployment

### Using the Official Image

```bash
docker run -d \
  --name faucet \
  -p 8080:8080 \
  -v faucet-data:/data \
  faucetdb/faucet:latest
```

### Docker Compose

```yaml
version: "3.9"

services:
  faucet:
    image: faucetdb/faucet:latest
    ports:
      - "8080:8080"
    volumes:
      - faucet-data:/data
    restart: unless-stopped
    # The image has a built-in HEALTHCHECK, no need to override

volumes:
  faucet-data:
```

### Building a Custom Image

The included multi-stage `Dockerfile` builds both the UI and the Go binary:

```dockerfile
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
CMD ["serve", "--host", "0.0.0.0"]
```

Build it:

```bash
docker build -t faucet:custom .
```

The final image is built `FROM alpine:3.21` with a non-root `faucet` user, a built-in health check, and `FAUCET_DATA_DIR=/data` preconfigured. Data persists across restarts via the `/data` volume.

---

## Kubernetes / Helm

### Basic Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: faucet
  labels:
    app: faucet
spec:
  replicas: 1
  selector:
    matchLabels:
      app: faucet
  template:
    metadata:
      labels:
        app: faucet
    spec:
      containers:
        - name: faucet
          image: faucetdb/faucet:latest
          args: ["serve", "--host", "0.0.0.0", "--data-dir", "/data"]
          ports:
            - containerPort: 8080
              name: http
          volumeMounts:
            - name: data
              mountPath: /data
          livenessProbe:
            httpGet:
              path: /healthz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: http
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              cpu: 100m
              memory: 64Mi
            limits:
              cpu: "1"
              memory: 256Mi
          env:
            - name: FAUCET_AUTH_JWT_SECRET
              valueFrom:
                secretKeyRef:
                  name: faucet-secrets
                  key: jwt-secret
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: faucet-data
---
apiVersion: v1
kind: Service
metadata:
  name: faucet
spec:
  selector:
    app: faucet
  ports:
    - port: 80
      targetPort: http
      name: http
  type: ClusterIP
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: faucet-data
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: Secret
metadata:
  name: faucet-secrets
type: Opaque
stringData:
  jwt-secret: "your-production-jwt-secret-here"
```

### Ingress

```yaml
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: faucet
  annotations:
    nginx.ingress.kubernetes.io/proxy-body-size: "10m"
spec:
  rules:
    - host: api.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: faucet
                port:
                  number: 80
  tls:
    - hosts:
        - api.example.com
      secretName: faucet-tls
```

### Important Notes for Kubernetes

- Faucet stores its configuration in a SQLite database. If you run multiple replicas, each needs its own PVC (or use an external config database).
- For horizontal scaling, consider running a single Faucet instance as the control plane and putting a load balancer in front of the database connections directly, or use an external PostgreSQL database for Faucet's config store (future feature).

---

## systemd Service

Create `/etc/systemd/system/faucet.service`:

```ini
[Unit]
Description=Faucet REST API Server
After=network.target

[Service]
Type=simple
User=faucet
Group=faucet
ExecStart=/usr/local/bin/faucet serve --host 0.0.0.0 --port 8080 --data-dir /var/lib/faucet
Restart=on-failure
RestartSec=5
LimitNOFILE=65536

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/faucet
PrivateTmp=true

# Environment
Environment=FAUCET_AUTH_JWT_SECRET=your-production-secret

[Install]
WantedBy=multi-user.target
```

Set up:

```bash
# Create the user and data directory
sudo useradd -r -s /bin/false faucet
sudo mkdir -p /var/lib/faucet
sudo chown faucet:faucet /var/lib/faucet

# Copy the binary
sudo cp ./bin/faucet /usr/local/bin/faucet

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable faucet
sudo systemctl start faucet

# Check status
sudo systemctl status faucet
sudo journalctl -u faucet -f
```

---

## Reverse Proxy

### nginx

```nginx
upstream faucet {
    server 127.0.0.1:8080;
    keepalive 32;
}

server {
    listen 443 ssl http2;
    server_name api.example.com;

    ssl_certificate     /etc/ssl/certs/api.example.com.crt;
    ssl_certificate_key /etc/ssl/private/api.example.com.key;

    # Security headers
    add_header Strict-Transport-Security "max-age=31536000; includeSubDomains" always;
    add_header X-Content-Type-Options nosniff always;
    add_header X-Frame-Options DENY always;

    # Request size limit
    client_max_body_size 10m;

    location / {
        proxy_pass http://faucet;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # Timeouts
        proxy_connect_timeout 10s;
        proxy_send_timeout 60s;
        proxy_read_timeout 60s;

        # WebSocket / SSE support (for NDJSON streaming)
        proxy_http_version 1.1;
        proxy_set_header Connection "";
        proxy_buffering off;
    }

    # Health check endpoint (no access log)
    location /healthz {
        proxy_pass http://faucet;
        access_log off;
    }
}

# HTTP redirect
server {
    listen 80;
    server_name api.example.com;
    return 301 https://$server_name$request_uri;
}
```

### Caddy

```
api.example.com {
    reverse_proxy localhost:8080

    # Optional: rate limiting
    # rate_limit {remote.ip} 100r/m

    header {
        Strict-Transport-Security "max-age=31536000; includeSubDomains"
        X-Content-Type-Options nosniff
        X-Frame-Options DENY
    }
}
```

Caddy automatically provisions and renews TLS certificates via Let's Encrypt.

---

## Environment Variables

All configuration can be set via environment variables with the `FAUCET_` prefix. Nested keys use underscores:

| Environment Variable | Config Key | Description | Default |
|---------------------|------------|-------------|---------|
| `FAUCET_SERVER_PORT` | `server.port` | HTTP listen port | `8080` |
| `FAUCET_SERVER_HOST` | `server.host` | HTTP listen host | `0.0.0.0` |
| `FAUCET_AUTH_JWT_SECRET` | `auth.jwt_secret` | JWT signing secret | `faucet-dev-secret-change-me` |

### Setting in Different Environments

**Shell:**

```bash
export FAUCET_AUTH_JWT_SECRET="your-production-secret"
export FAUCET_SERVER_PORT=9090
faucet serve
```

**Docker:**

```bash
docker run -d \
  -e FAUCET_AUTH_JWT_SECRET="your-production-secret" \
  -e FAUCET_SERVER_PORT=8080 \
  -p 8080:8080 \
  faucetdb/faucet:latest serve
```

**Docker Compose:**

```yaml
services:
  faucet:
    image: faucetdb/faucet:latest
    environment:
      FAUCET_AUTH_JWT_SECRET: "${JWT_SECRET}"
      FAUCET_SERVER_PORT: "8080"
```

**Kubernetes:**

```yaml
env:
  - name: FAUCET_AUTH_JWT_SECRET
    valueFrom:
      secretKeyRef:
        name: faucet-secrets
        key: jwt-secret
```

---

## Production Checklist

Before deploying Faucet to production, verify each item:

### Security

- [ ] **Change the JWT secret.** The default value `faucet-dev-secret-change-me` is insecure. Set `FAUCET_AUTH_JWT_SECRET` to a strong random string (at least 32 characters).
- [ ] **Create a strong admin password.** Use at least 16 characters with mixed case, numbers, and symbols.
- [ ] **Use TLS.** Terminate TLS at your reverse proxy (nginx, Caddy) or load balancer. Never expose Faucet over plain HTTP in production.
- [ ] **Restrict network access.** Bind Faucet to `127.0.0.1` if it is behind a reverse proxy on the same host.
- [ ] **Use API keys for service consumers.** Do not share admin JWT tokens with application code. Create role-scoped API keys instead.
- [ ] **Review RBAC roles.** Follow the principle of least privilege. Grant only the verb mask and table access each consumer needs.
- [ ] **Set services to read-only where appropriate.** Analytics databases and replicas should have `read_only: true`.
- [ ] **Disable raw SQL unless needed.** Only enable `raw_sql_allowed` on services where AI agents or advanced users genuinely need it.

### Database Connections

- [ ] **Use connection strings with SSL/TLS.** Set `sslmode=require` (PostgreSQL), `tls=true` (MySQL), `encrypt=true` (SQL Server).
- [ ] **Use dedicated database users.** Create a database user specifically for Faucet with the minimum required privileges.
- [ ] **Tune connection pools.** Adjust `max_open_conns` based on your database server's connection limits and expected traffic.
- [ ] **Test connections before going live.** Use `faucet db test <name>` to verify each service connects successfully.

### Infrastructure

- [ ] **Set up health checks.** Point your load balancer or orchestrator at `/healthz` (liveness) and `/readyz` (readiness).
- [ ] **Configure log aggregation.** Faucet logs to stderr in structured text format. Forward to your log aggregation system.
- [ ] **Set resource limits.** In Kubernetes, set CPU and memory requests/limits. Faucet is lightweight -- 64 MB memory is usually sufficient.
- [ ] **Back up the data directory.** The SQLite config store at `~/.faucet` (or `--data-dir`) contains all service, role, API key, and admin configurations.
- [ ] **Set up monitoring.** Monitor the `/healthz` endpoint and track API latency via your reverse proxy or APM tool.

### Operational

- [ ] **Pin the image version.** Use `faucetdb/faucet:v0.1.0` instead of `:latest` in production deployments.
- [ ] **Set up automatic restarts.** Use `restart: unless-stopped` in Docker Compose, `Restart=on-failure` in systemd, or Kubernetes restart policies.
- [ ] **Plan for upgrades.** Faucet is a single binary. Upgrades are as simple as replacing the binary and restarting.
- [ ] **Document your services.** Keep a record of which database services are configured, their roles, and who holds each API key.

---

## Graceful Shutdown

Faucet handles `SIGINT` and `SIGTERM` signals for graceful shutdown:

1. Stops accepting new connections
2. Drains in-flight requests (30-second timeout)
3. Closes all database connections
4. Exits cleanly

This makes Faucet compatible with rolling deployments in Kubernetes and zero-downtime restarts with systemd.

---

## Server Timeouts

The HTTP server has these timeouts:

| Timeout | Value | Description |
|---------|-------|-------------|
| Read timeout | 15 seconds | Max time to read the full request |
| Write timeout | 60 seconds | Max time to write the full response |
| Idle timeout | 120 seconds | Max time for keep-alive connections |
| Shutdown timeout | 30 seconds | Max time to drain in-flight requests |

These are suitable for most workloads. Long-running queries should use the `timeout` parameter in raw SQL calls rather than relying on HTTP timeouts.
