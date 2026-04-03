# Phase 7 — Deployment

## Status

- [x] Move `web/` to `server/web/`; update README dev command
- [x] `server/static.go` — `go:embed` for self-contained binary
- [x] `server/main.go` — serve embedded assets when `--static` is not set; add `GIFT_EXCHANGE_*` env var fallbacks
- [x] `Dockerfile` — multi-stage build (project root)
- [x] `helm/gift-exchange/` — Helm chart (Chart.yaml, values.yaml, templates/)
- [x] Smoke test: Docker container serves frontend; `helm template` renders cleanly

## Goal

Package the server as a self-contained Docker image and provide a publishable Helm chart for Kubernetes deployment. The server is stateless and auth-agnostic — a Forward Auth proxy handles authentication entirely external to the application.

---

## 1. Static File Embedding

### 1.1 Motivation

Production should be a single self-contained binary — no `--static` flag, no volume mount, no ConfigMap for static assets. `go:embed` achieves this. Development retains the `--static server/web/` escape hatch for live file editing without rebuilding.

### 1.2 Directory Move: `web/` → `server/web/`

`go:embed` paths are relative to the file containing the directive and cannot use `..` traversal. Since the directive lives in `server/static.go`, the assets must reside under `server/`. Move `web/` to `server/web/`.

Update the README development command:

```bash
# Development (live files, no rebuild):
go run ./server/ --static server/web/

# Production / embedded (no flag needed):
go run ./server/
```

### 1.3 Implementation

**New file `server/static.go`:**

```go
package main

import "embed"

//go:embed web
var embeddedWeb embed.FS
```

**`server/main.go` — `newServer()` change.** When `cfg.staticDir` is empty, serve from the embedded FS:

```go
import "io/fs"

if cfg.staticDir != "" {
    mux.Handle("/", http.FileServer(http.Dir(cfg.staticDir)))
} else {
    sub, _ := fs.Sub(embeddedWeb, "web")
    mux.Handle("/", http.FileServer(http.FS(sub)))
}
```

The `--static` flag default changes from `""` (no static serving) to `""` (serve embedded). The behavior is identical for development users who pass `--static`.

---

## 2. Dockerfile

**Location:** `Dockerfile` (project root). Build from the project root:

```bash
docker build -t gift-exchange:latest .
```

### 2.1 Multi-Stage Build

```dockerfile
# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build \
      -ldflags="-s -w" \
      -o /gift-exchange \
      ./server/

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM gcr.io/distroless/static:nonroot
COPY --from=builder /gift-exchange /gift-exchange
EXPOSE 8080
ENTRYPOINT ["/gift-exchange"]
```

**Key decisions:**

| Choice             | Decision                           | Rationale                                                                                                                                    |
| ------------------ | ---------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------- |
| Runtime base       | `gcr.io/distroless/static:nonroot` | No shell, minimal attack surface; `:nonroot` runs as uid 65532 without a USER instruction; no Debian version pin so it receives base updates |
| CGO                | Disabled                           | Go stdlib + embed; no cgo needed; produces a true static binary                                                                              |
| No CMD             | Intentional                        | All configuration is passed as `GIFT_EXCHANGE_*` env vars in the k8s Deployment spec                                                        |
| `-ldflags="-s -w"` | Yes                                | Strips DWARF debug info and Go symbol table; ~30% smaller binary                                                                             |

**`.dockerignore`** (at project root):

```
experiments/
plans/
giftexchange.py
participants.json
relationships.json
history.json
.git/
```

---

## 3. Helm Chart

**Location:** `helm/gift-exchange/` (project root)

```
helm/
└── gift-exchange/
    ├── Chart.yaml
    ├── values.yaml
    └── templates/
        ├── _helpers.tpl
        ├── deployment.yaml
        ├── service.yaml
        └── ingress.yaml
```

### 3.1 `Chart.yaml`

`version` and `appVersion` are kept in sync — one app, one chart. `image.tag` defaults to `appVersion` (see §3.4).

```yaml
apiVersion: v2
name: gift-exchange
description: Optimized gift exchange assignment tool
type: application
version: 0.1.0
appVersion: "0.1.0"
```

### 3.2 `values.yaml`

```yaml
replicaCount: 1

image:
  repository: ghcr.io/cbochs/gift-exchange
  tag: ""           # empty = use Chart.appVersion
  pullPolicy: IfNotPresent

server:
  corsOrigin: ""    # REQUIRED — https://gift-exchange.example.com
  timeout: "15s"

ingress:
  enabled: true
  className: "traefik"  # or "nginx"
  hostname: ""      # REQUIRED — gift-exchange.example.com
  tls: true
  annotations: {}   # e.g. Traefik forward-auth, cert-manager

resources:
  requests:
    cpu: 50m
    memory: 32Mi
  limits:
    cpu: 200m
    memory: 64Mi
```

### 3.3 `templates/_helpers.tpl`

```
{{- define "gift-exchange.name" -}}{{ .Chart.Name }}{{- end }}

{{- define "gift-exchange.labels" -}}
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version }}
app.kubernetes.io/name: {{ include "gift-exchange.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "gift-exchange.selectorLabels" -}}
app.kubernetes.io/name: {{ include "gift-exchange.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}
```

### 3.4 `templates/deployment.yaml`

Configuration is passed via `env:` rather than `args:`. This aligns with standard k8s practice and makes values overridable via ConfigMaps without rewriting the Deployment spec. `image.tag` falls back to `Chart.appVersion` when left empty.

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "gift-exchange.name" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "gift-exchange.labels" . | nindent 4 }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels: {{- include "gift-exchange.selectorLabels" . | nindent 6 }}
  template:
    metadata:
      labels: {{- include "gift-exchange.selectorLabels" . | nindent 8 }}
    spec:
      containers:
        - name: gift-exchange
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag | default .Chart.AppVersion }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          env:
            - name: GIFT_EXCHANGE_CORS_ORIGIN
              value: "{{ .Values.server.corsOrigin }}"
            - name: GIFT_EXCHANGE_TIMEOUT
              value: "{{ .Values.server.timeout }}"
          ports:
            - containerPort: 8080
          resources: {{- toYaml .Values.resources | nindent 12 }}
          livenessProbe:
            httpGet:
              path: /api/v1/health
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 15
          readinessProbe:
            httpGet:
              path: /api/v1/health
              port: 8080
            initialDelaySeconds: 2
            periodSeconds: 10
          securityContext:
            readOnlyRootFilesystem: true
            allowPrivilegeEscalation: false
            runAsNonRoot: true
```

### 3.5 `templates/service.yaml`

```yaml
apiVersion: v1
kind: Service
metadata:
  name: {{ include "gift-exchange.name" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "gift-exchange.labels" . | nindent 4 }}
spec:
  selector: {{- include "gift-exchange.selectorLabels" . | nindent 4 }}
  ports:
    - port: 80
      targetPort: 8080
  type: ClusterIP
```

### 3.6 `templates/ingress.yaml`

Uses standard `networking.k8s.io/v1 Ingress` for portability across Traefik, nginx, and others. Traefik processes standard Ingress resources alongside its CRD routes — forward-auth middleware is attached via annotations (see §4).

```yaml
{{- if .Values.ingress.enabled }}
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: {{ include "gift-exchange.name" . }}
  namespace: {{ .Release.Namespace }}
  labels: {{- include "gift-exchange.labels" . | nindent 4 }}
  {{- with .Values.ingress.annotations }}
  annotations: {{- toYaml . | nindent 4 }}
  {{- end }}
spec:
  ingressClassName: {{ .Values.ingress.className }}
  {{- if .Values.ingress.tls }}
  tls:
    - hosts:
        - {{ .Values.ingress.hostname }}
  {{- end }}
  rules:
    - host: {{ .Values.ingress.hostname }}
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: {{ include "gift-exchange.name" . }}
                port:
                  number: 80
{{- end }}
```

### 3.7 Publishing

Package and push to an OCI registry (Helm 3.8+):

```bash
helm package helm/gift-exchange
helm push gift-exchange-0.1.0.tgz oci://ghcr.io/cbochs/charts
```

Install from the registry:

```bash
helm install gift-exchange oci://ghcr.io/cbochs/charts/gift-exchange \
  --namespace gift-exchange --create-namespace \
  --set image.repository=ghcr.io/cbochs/gift-exchange \
  --set server.corsOrigin=https://gift-exchange.example.com \
  --set ingress.hostname=gift-exchange.example.com
```

Note: `--set image.tag` is omitted — the chart uses `Chart.appVersion` by default.

---

## 4. Forward Auth

The server has no authentication code. All requests reaching it are pre-authenticated by the proxy. No headers are read, no tokens are validated, no sessions exist.

Configure the auth middleware via `ingress.annotations` in `values.yaml`. Example with Traefik ForwardAuth (tinyauth pattern from home-server):

```yaml
ingress:
  annotations:
    traefik.ingress.kubernetes.io/router.middlewares: "tinyauth-forward-auth@kubernetescrd"
```

### 4.1 Health Probes and Auth

k8s liveness and readiness probes originate from the kubelet and hit the **pod IP directly** — they do not go through the ingress or any proxy middleware. The health endpoint (`/api/v1/health`) is always reachable by k8s probes regardless of ingress auth configuration. No special ingress path exceptions are needed.

### 4.2 Redirect URLs

This app has no OAuth flows, no login callbacks, no session state. No redirect URLs need to be configured anywhere in the application. The hostname only appears in `GIFT_EXCHANGE_CORS_ORIGIN` and the Ingress `host:` field.

---

## 5. Configuration Reference

Server configuration is read from `GIFT_EXCHANGE_*` environment variables. CLI flags take precedence over env vars; env vars take precedence over hardcoded defaults. This allows k8s Deployments to configure via `env:` without modifying the Deployment spec between environments.

| Flag            | Env var                    | Default    | k8s value                                           |
| --------------- | -------------------------- | ---------- | --------------------------------------------------- |
| `--addr`        | `GIFT_EXCHANGE_ADDR`       | `:8080`    | `:8080` (usually left as default)                   |
| `--cors-origin` | `GIFT_EXCHANGE_CORS_ORIGIN`| `*`        | Set to `https://<hostname>` in production           |
| `--timeout`     | `GIFT_EXCHANGE_TIMEOUT`    | `15s`      | `15s` (increase if solver is slow for large groups) |
| `--static`      | `GIFT_EXCHANGE_STATIC`     | (embedded) | Not set in production; `server/web/` in development |

Implementation pattern in `server/main.go`:

```go
func envOrDefault(key, fallback string) string {
    if v := os.Getenv("GIFT_EXCHANGE_" + key); v != "" {
        return v
    }
    return fallback
}
```

Flags are defined with `envOrDefault` as the default value so that an explicit flag still wins.

---

## 6. Implementation Plan

1. Move `web/` to `server/web/`. Update `README.md`. Verify `go build ./server/` and all tests pass.
2. Create `server/static.go`. Update `server/main.go`: serve embedded FS when `--static` is empty; add `GIFT_EXCHANGE_*` env var fallbacks for all flags. Test: `go run ./server/` serves the frontend.
3. Write `Dockerfile` and `.dockerignore` at project root. Test: `docker build -t gift-exchange:latest .` succeeds; `docker run -p 8080:8080` serves the frontend.
4. Write Helm chart (`Chart.yaml`, `values.yaml`, four templates). Test: `helm template gift-exchange helm/gift-exchange --set image.repository=x --set server.corsOrigin=https://x --set ingress.hostname=x` renders valid YAML.
5. Deploy to target cluster. Verify health probe, UI access through auth proxy, solve request.
