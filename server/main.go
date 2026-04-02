package main

import (
	"flag"
	"fmt"
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
	}

	var chain http.Handler = mux
	chain = corsMiddleware(cfg.corsOrigin, chain)
	if cfg.timeout > 0 {
		chain = http.TimeoutHandler(chain, cfg.timeout, `{"feasible":false,"error":"request timeout"}`)
	}
	return chain
}

func main() {
	var cfg serverConfig
	flag.StringVar(&cfg.addr, "addr", ":8080", "listen address")
	flag.StringVar(&cfg.corsOrigin, "cors-origin", "*", "allowed CORS origin")
	flag.DurationVar(&cfg.timeout, "timeout", 15*time.Second, "request timeout")
	flag.StringVar(&cfg.staticDir, "static", "", "directory to serve static frontend files")
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
