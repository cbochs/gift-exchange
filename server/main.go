package server

import (
	"io/fs"
	"net/http"
	"time"
)

// Config holds the server configuration.
type Config struct {
	Addr       string
	CORSOrigin string
	Timeout    time.Duration
	StaticDir  string
}

const (
	DefaultAddr       = ":8080"
	DefaultCORSOrigin = "*"
	DefaultTimeout    = 15 * time.Second

	// ReadTimeout, WriteTimeoutBuffer, and IdleTimeout are http.Server
	// transport layer limits used by cmd/server/main.go.
	ReadTimeout        = 5 * time.Second
	WriteTimeoutBuffer = 5 * time.Second // added to solver Timeout for WriteTimeout
	IdleTimeout        = 60 * time.Second

	// maxRequestBodyBytes caps incoming request bodies at 1 MB.
	maxRequestBodyBytes = 1 << 20

	// defaultTimeout is used by dtoToProblem when no per-request timeout is set.
	defaultTimeout = DefaultTimeout
)

// ParseDuration is a convenience wrapper for time.ParseDuration, exposed so
// cmd/server/main.go can parse the GIFT_EXCHANGE_TIMEOUT env var without
// importing time directly alongside server constants.
func ParseDuration(s string) (time.Duration, error) {
	return time.ParseDuration(s)
}

// NewServer constructs the HTTP handler chain for the gift exchange server.
func NewServer(cfg Config) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/solve", solveHandler)
	mux.HandleFunc("GET /api/v1/health", healthHandler)

	if cfg.StaticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(cfg.StaticDir)))
	} else {
		sub, _ := fs.Sub(embeddedWeb, "web")
		mux.Handle("/", http.FileServer(http.FS(sub)))
	}

	var chain http.Handler = mux
	chain = corsMiddleware(cfg.CORSOrigin, chain)
	if cfg.Timeout > 0 {
		chain = http.TimeoutHandler(chain, cfg.Timeout, `{"feasible":false,"error":"request timeout"}`)
	}
	return chain
}
