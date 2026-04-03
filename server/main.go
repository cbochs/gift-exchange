package main

import (
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"time"
)

type serverConfig struct {
	addr       string
	corsOrigin string
	timeout    time.Duration
	staticDir  string
}

func newServer(cfg serverConfig) http.Handler {
	h := newHandler()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/solve", h.solveHandler)
	mux.HandleFunc("/api/v1/health", h.healthHandler)

	if cfg.staticDir != "" {
		mux.Handle("/", http.FileServer(http.Dir(cfg.staticDir)))
	} else {
		sub, _ := fs.Sub(embeddedWeb, "web")
		mux.Handle("/", http.FileServer(http.FS(sub)))
	}

	var chain http.Handler = mux
	chain = corsMiddleware(cfg.corsOrigin, chain)
	if cfg.timeout > 0 {
		chain = http.TimeoutHandler(chain, cfg.timeout, `{"feasible":false,"error":"request timeout"}`)
	}
	return chain
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv("GIFT_EXCHANGE_" + key); v != "" {
		return v
	}
	return fallback
}

func main() {
	var cfg serverConfig
	flag.StringVar(&cfg.addr, "addr", envOrDefault("ADDR", ":8080"), "listen address")
	flag.StringVar(&cfg.corsOrigin, "cors-origin", envOrDefault("CORS_ORIGIN", "*"), "allowed CORS origin")
	flag.StringVar(&cfg.staticDir, "static", envOrDefault("STATIC", ""), "directory to serve static frontend files (default: embedded)")

	timeoutStr := envOrDefault("TIMEOUT", "15s")
	defaultTimeout, err := time.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid GIFT_EXCHANGE_TIMEOUT %q: %v\n", timeoutStr, err)
		os.Exit(1)
	}
	flag.DurationVar(&cfg.timeout, "timeout", defaultTimeout, "request timeout")

	flag.Parse()

	srv := &http.Server{
		Addr:    cfg.addr,
		Handler: newServer(cfg),
	}

	fmt.Fprintf(os.Stderr, "listening on %s\n", cfg.addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
