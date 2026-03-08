package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"hexsonic/internal/auth"
	"hexsonic/internal/config"
	"hexsonic/internal/db"
	"hexsonic/internal/httpapi"
	"hexsonic/internal/security"
	"hexsonic/internal/storage"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	pool, err := pgxpool.New(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer pool.Close()

	if err := db.Migrate(context.Background(), pool); err != nil {
		log.Fatalf("migrate: %v", err)
	}

	store, err := storage.New(cfg.StorageRoot)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}

	signer := security.NewSigner(cfg.SigningKey)

	var verifier *auth.Verifier
	if cfg.AuthRequired {
		v, err := auth.NewVerifier(context.Background(), cfg.OIDCIssuerURL, cfg.OIDCAudience)
		if err != nil {
			log.Fatalf("oidc verifier: %v", err)
		}
		verifier = v
	}
	srv := &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      httpapi.New(cfg, pool, store, signer, verifier).Router(),
		ReadTimeout:  cfg.HTTPReadTimeout,
		WriteTimeout: cfg.HTTPWriteTimeout,
		IdleTimeout:  cfg.HTTPIdleTimeout,
	}

	go func() {
		log.Printf("HEXSONIC API listening on %s", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
