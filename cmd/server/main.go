package main

import (
	"flag"
	"fmt"
	"net/http"
	"os"

	"github.com/cbochs/gift-exchange/server"
)

func envOrDefault(key, fallback string) string {
	if v := os.Getenv("GIFT_EXCHANGE_" + key); v != "" {
		return v
	}
	return fallback
}

func main() {
	var cfg server.Config
	flag.StringVar(&cfg.Addr, "addr", envOrDefault("ADDR", server.DefaultAddr), "listen address")
	flag.StringVar(&cfg.CORSOrigin, "cors-origin", envOrDefault("CORS_ORIGIN", server.DefaultCORSOrigin), "allowed CORS origin")
	flag.StringVar(&cfg.StaticDir, "static", envOrDefault("STATIC", ""), "directory to serve static frontend files (default: embedded)")

	timeoutStr := envOrDefault("TIMEOUT", server.DefaultTimeout.String())
	parsedTimeout, err := server.ParseDuration(timeoutStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "invalid GIFT_EXCHANGE_TIMEOUT %q: %v\n", timeoutStr, err)
		os.Exit(1)
	}
	flag.DurationVar(&cfg.Timeout, "timeout", parsedTimeout, "request timeout")

	flag.Parse()

	srv := &http.Server{
		Addr:         cfg.Addr,
		Handler:      server.NewServer(cfg),
		ReadTimeout:  server.ReadTimeout,
		WriteTimeout: cfg.Timeout + server.WriteTimeoutBuffer,
		IdleTimeout:  server.IdleTimeout,
	}

	fmt.Fprintf(os.Stderr, "listening on %s\n", cfg.Addr)
	if err := srv.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "server error: %v\n", err)
		os.Exit(1)
	}
}
