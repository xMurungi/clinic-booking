package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/xMurungi/clinic-booking/internal/httpx"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}


func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}

func run() error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Healthz)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpx.Chain(mux, httpx.RequestID, httpx.Logging, httpx.Recover),
		ReadHeaderTimeout: 5 * time.Second,
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutDownTimeout)
	defer cancel()
	
	return srv.Shutdown(shutdownCtx)
}
