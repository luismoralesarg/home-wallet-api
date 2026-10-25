package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/luismoralesarg/home-wallet-api/internal/adapters/dolarapi"
	"github.com/luismoralesarg/home-wallet-api/internal/adapters/http"
	"github.com/luismoralesarg/home-wallet-api/internal/adapters/jwt"
	"github.com/luismoralesarg/home-wallet-api/internal/adapters/postgres"
	"github.com/luismoralesarg/home-wallet-api/internal/application"
	"github.com/luismoralesarg/home-wallet-api/internal/config"
	"github.com/luismoralesarg/home-wallet-api/internal/scheduler"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	auth := application.NewAuthService(postgres.NewUserRepo(pool), jwt.New(cfg.JWTSecret, cfg.JWTTTL))
	fx := application.NewExchangeRateService(postgres.NewExchangeRateRepo(pool), dolarapi.New(cfg.DolarAPIURL))
	app := http.NewApp(http.Deps{Auth: auth, ExchangeRate: fx})

	// Cotización: al arrancar si falta la de hoy, y todos los días a las 09:00 (Mendoza).
	if err := fx.EnsureToday(ctx); err != nil {
		slog.Error("no se pudo obtener la cotización inicial", "err", err)
	}
	go scheduler.RunDaily(ctx, 9, fx.RefreshOrLog)

	go func() {
		<-ctx.Done()
		_ = app.Shutdown()
	}()
	slog.Info("escuchando", "port", cfg.Port)
	return app.Listen(":" + cfg.Port)
}
