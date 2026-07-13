package main

import (
	"context"
	"fmt"
	"log"
	"messenger/internal/core/config"
	"messenger/internal/core/features/auth"
	"messenger/internal/core/logger"
	"messenger/internal/db"

	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"
)

func main() {
	_ = godotenv.Load()

	cfg := config.NewConfigMust()

	appLogger, err := logger.NewLogger()
	if err != nil {
		fmt.Println("failed to init application logger:", err)
		os.Exit(1)
	}
	defer appLogger.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DBSource, appLogger.Logger)
	if err != nil {
		appLogger.Fatal("failed to init postgres connection pool", zap.Error(err))
	}
	defer pool.Close()

	authRepository := auth.NewRepository(pool)
	authService := auth.NewService(authRepository)
	authHandler := auth.NewHandler(authService)

	mux := http.NewServeMux()

	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		appLogger.Info("HTTP server started", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			appLogger.Error("HTTP server run error", zap.Error(err))
			log.Printf("HTTP server run error: %v", err)
		}
	}()

	<-ctx.Done()
	appLogger.Info("shutting down HTTP server...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		appLogger.Error("HTTP server forced to shutdown", zap.Error(err))
	}

	appLogger.Info("server stopped gracefully")
}
