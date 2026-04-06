# Phase 7 — Deployment

## Status

- [x] Move `web/` to `server/web/`; update README dev command
- [x] `server/static.go` — `go:embed` for self-contained binary
- [x] `server/main.go` — serve embedded assets when `--static` is not set; add `GIFT_EXCHANGE_*` env var fallbacks
- [x] Dagger module — build, serve, and publish multi-arch container image
- [x] Smoke test: container serves frontend; `Publish` pushes to ghcr.io

## Goal

Package the server as a self-contained container image and publish it to ghcr.io. The server is stateless and auth-agnostic — a Forward Auth proxy handles authentication entirely external to the application.

---

## 1. Static File Embedding

### 1.1 Motivation

Production should be a single self-contained binary — no `--static` flag, no volume mount, no ConfigMap for static assets. `go:embed` achieves this. Development retains the `--static server/web/` escape hatch for live file editing without rebuilding.

### 1.2 Directory Move: `web/` → `server/web/`

`go:embed` paths are relative to the file containing the directive and cannot use `..` traversal. Since the directive lives in `server/static.go`, the assets must reside under `server/`. Move `web/` to `server/web/`.

Development and production invocations:

```bash
# Development (live files, no rebuild):
go run ./server/ --static server/web/

# Production / embedded (no flag needed):
go run ./server/
```

### 1.3 Implementation

**`server/static.go`:**

```go
package main

import "embed"

//go:embed web
var embeddedWeb embed.FS
```

**`server/main.go` — `newServer()` change.** When `cfg.staticDir` is empty, serve from the embedded FS:

```go
if cfg.staticDir != "" {
    mux.Handle("/", http.FileServer(http.Dir(cfg.staticDir)))
} else {
    sub, _ := fs.Sub(embeddedWeb, "web")
    mux.Handle("/", http.FileServer(http.FS(sub)))
}
```

---

## 2. Dagger Build Pipeline

**Location:** `.dagger/` (Dang SDK)

```
dagger.json                  ← module root; declares "go" local dependency
.dagger/
├── config.toml              ← module config (go = "modules/go")
├── main.dang                ← GiftExchange type: Container, Serve, Publish
└── modules/
    └── go/
        ├── dagger.json
        └── main.dang        ← Go type: Build (cross-compile helper)
```

### 2.1 Go Module (`modules/go/main.dang`)

Wraps `go build` with cross-compilation support. Uses a pinned `golang:1.26` image digest for reproducibility. Outputs a single static binary file.

```
go.Build(source, pkg, platform) → File
```

### 2.2 GiftExchange Module (`.dagger/main.dang`)

Three exported functions:

| Function | Description |
| --- | --- |
| `Container(platform)` | Builds the binary via `go.Build`, adds it to a bare container with exec permissions. |
| `Serve()` | Wraps `Container` as a `Service` for local testing. |
| `Publish(tag, username, password, registry, repo)` | Builds `linux/amd64` and `linux/arm64` variants, publishes a multi-arch manifest to `registry/repo:tag`. Defaults to `ghcr.io/cbochs/gift-exchange`. |

### 2.3 Publishing

```bash
dagger call publish \
  --tag v0.1.0 \
  --username cbochs \
  --password env:GITHUB_TOKEN
```

This pushes a multi-arch image manifest to `ghcr.io/cbochs/gift-exchange:v0.1.0`.

---

## 3. Forward Auth

The server has no authentication code. All requests reaching it are pre-authenticated by the proxy. No headers are read, no tokens are validated, no sessions exist.

### 3.1 Health Probes and Auth

k8s liveness and readiness probes originate from the kubelet and hit the **pod IP directly** — they do not go through the ingress or any proxy middleware. The health endpoint (`/api/v1/health`) is always reachable by k8s probes regardless of ingress auth configuration. No special ingress path exceptions are needed.

### 3.2 Redirect URLs

This app has no OAuth flows, no login callbacks, no session state. No redirect URLs need to be configured anywhere in the application. The hostname only appears in `GIFT_EXCHANGE_CORS_ORIGIN` and the ingress host field.

---

## 4. Configuration Reference

Server configuration is read from `GIFT_EXCHANGE_*` environment variables. CLI flags take precedence over env vars; env vars take precedence over hardcoded defaults.

| Flag            | Env var                     | Default    | Notes                                               |
| --------------- | --------------------------- | ---------- | --------------------------------------------------- |
| `--addr`        | `GIFT_EXCHANGE_ADDR`        | `:8080`    | Usually left as default in k8s                      |
| `--cors-origin` | `GIFT_EXCHANGE_CORS_ORIGIN` | `*`        | Set to `https://<hostname>` in production           |
| `--timeout`     | `GIFT_EXCHANGE_TIMEOUT`     | `15s`      | Increase if solver is slow for large groups         |
| `--static`      | `GIFT_EXCHANGE_STATIC`      | (embedded) | Not set in production; `server/web/` in development |

Implementation pattern in `server/main.go`:

```go
func envOrDefault(key, fallback string) string {
    if v := os.Getenv("GIFT_EXCHANGE_" + key); v != "" {
        return v
    }
    return fallback
}
```
