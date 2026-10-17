package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
	_ "time/tzdata"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/pgx/v5"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/xMurungi/clinic-booking/internal/booking"
	"github.com/xMurungi/clinic-booking/internal/config"
	"github.com/xMurungi/clinic-booking/internal/handlers"
	"github.com/xMurungi/clinic-booking/internal/httpx"
	"github.com/xMurungi/clinic-booking/internal/store"
	"github.com/xMurungi/clinic-booking/migrations"
)

func Healthz(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := pgxpool.New(ctx, cfg.DatabaseUrl)

	if err != nil {
		return fmt.Errorf("create pool: %w", err)
	}
	defer pool.Close()

	// if err := pool.Ping(ctx); err != nil {
	// 	return fmt.Errorf("ping db: %w", err)
	// }

	if err := waitForDB(ctx, pool, 30*time.Second); err != nil {
		return fmt.Errorf("ping db: %w", err)
	}

	if err := runMigrations(cfg.DatabaseUrl); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}

	if err != nil {
		return err
	}
	defer pool.Close()

	loc, err := time.LoadLocation("Africa/Nairobi")
	if err != nil {
		return err
	}

	st := store.New(pool)
	svc := booking.NewService(st, loc, time.Now, time.Hour) // 1h lead time
	api := handlers.New(svc, loc)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", Healthz)
	mux.HandleFunc("GET /doctors/{id}/availability", api.Availability)
	mux.HandleFunc("POST /appointments", api.Book)
	mux.HandleFunc("PATCH /appointments/{id}/cancel", api.Cancel)
	mux.HandleFunc("PATCH /appointments/{id}/reschedule", api.Reschedule)
	mux.HandleFunc("GET /patients/{id}/appointments", api.PatientAppointments)

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpx.Chain(mux, httpx.RequestID, httpx.Logging, httpx.Recover),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.HTTPAddr)
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
	}

	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutDownTimeout)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}

func runMigrations(dbURL string) error {
	src, err := iofs.New(migrations.FS, ".")
	if err != nil {
		return err
	}
	u := dbURL
	for _, p := range []string{"postgresql://", "postgres://"} {
		if strings.HasPrefix(u, p) {
			u = "pgx5://" + strings.TrimPrefix(u, p)
			break
		}
	}
	m, err := migrate.NewWithSourceInstance("iofs", src, u)
	if err != nil {
		return err
	}
	defer m.Close()
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}

func waitForDB(ctx context.Context, pool *pgxpool.Pool, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var lastErr error
	for {
		pctx, pcancel := context.WithTimeout(ctx, 2*time.Second)
		lastErr = pool.Ping(pctx)
		pcancel()
		if lastErr == nil {
			return nil
		}
		slog.Warn("database not ready, retrying", "err", lastErr)

		select {
		case <-ctx.Done():
			return fmt.Errorf("gave up after %s: %w", timeout, lastErr)
		case <-time.After(time.Second):
		}
	}
}
