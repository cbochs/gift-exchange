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

## 2. Dagger Build and Release Pipeline

The repository uses Dagger v1 workspaces, pinned by `dagger.toml` and `dagger.lock`. CI runs with `DAGGER_X_RELEASE=v1.0.0-beta.25`.

```
dagger.toml
dagger.lock
.dagger/modules/
├── gift-exchange-build/      ← Builds CLI release binaries and server image binaries
├── gift-exchange-publish/    ← Publishes GitHub releases and GHCR images
├── platforms/                ← Parses and normalizes GOOS/GOARCH platform strings
└── semver/                   ← Validates and parses release tags
.github/workflows/
├── ci.yml                    ← Dagger checks on pull requests and main
└── release.yml               ← Publishes for pushed v* tags
```

### 2.1 Binary build

The `gift-exchange-build` module calls `dagger/go` to build both `./cmd/giftexchange` and `./cmd/server`. Its `binaries` function accepts separate platform lists for the CLI and server, defaulting to:

- `linux/amd64`
- `linux/arm64`
- `darwin/arm64` (macOS ARM64)

The CLI defaults to Linux `amd64`, Linux `arm64`, and macOS ARM64; the server defaults to Linux `amd64` and `arm64`. The publish module passes its configured binary platforms to the CLI build and image platforms to the server build.

The `platforms` module extracts the OS and architecture, maps common GNU architecture names (`amd64` → `x86_64`, `arm64` → `aarch64`), returns GNU triplets for Linux and macOS, and provides snake-case names such as `linux_amd64` for artifact paths.

### 2.2 Release publishing

The `semver` module validates SemVer 2.0.0 tags (with an optional leading `v`) and extracts version components and prerelease/build metadata. `gift-exchange-publish` rejects non-SemVer tags before publishing, then accepts the tag, GitHub token, GHCR username, and source commit SHA to:

- Builds and publishes server images for the configured Linux platforms as `ghcr.io/cbochs/gift-exchange:<tag>`.
- Adds OCI title, description, source, documentation, version, revision, and ref-name labels to the images.
- Creates a GitHub release for the same tag and attaches the CLI binaries for the configured binary platforms.
- Marks SemVer tags containing a prerelease suffix (for example, `-beta.1`) as prereleases; this does not affect whether the pipeline runs.

The release workflow is triggered by every pushed tag matching `v*`. It selects Dagger's `ci` environment (`dagger --env ci`), which sets the registry, repository, and platform lists in `dagger.toml`; Dagger combines the registry and repository into `<registry>/<repository>:<tag>`. CI permissions provide `contents: write` and `packages: write`, and credentials are supplied as secrets.

The Go runtime version is explicitly set to `1.27.3` for the Go, golangci-lint, and Staticcheck modules, so their project checks use `golang:1.27.3-alpine`. The upstream modules also have internal helper images (`golang:1.26-alpine`, `golang:1.27-alpine`, and a pinned golangci-lint tool image) that are not configurable through workspace settings. The release helper uses the latest Alpine image pinned as `alpine:3.24.3@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a`.

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
