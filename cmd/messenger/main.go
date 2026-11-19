package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"go.uber.org/zap"

	"messenger/internal/core/config"
	"messenger/internal/core/features/auth"
	"messenger/internal/core/features/chats"
	"messenger/internal/core/features/messages"
	"messenger/internal/core/features/upload"
	"messenger/internal/core/features/users"
	"messenger/internal/db"

	"messenger/internal/core/logger"
	"messenger/internal/core/redis"
	middleware "messenger/internal/core/transport/http/middleware"
	"messenger/internal/core/transport/ws"
)

func main() {
	_ = godotenv.Load()
	cfg, err := config.NewConfig()
	if err != nil {
		fmt.Println("configuration error:", err)
		os.Exit(1)
	}

	appLogger, err := logger.NewLogger()
	if err != nil {
		fmt.Println("failed to init logger:", err)
		os.Exit(1)
	}
	defer appLogger.Close()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	pool, err := db.NewPool(ctx, cfg.DBSource, appLogger.Logger)
	if err != nil {
		appLogger.Fatal("failed to init db", zap.Error(err))
	}
	defer pool.Close()

	redisClient := redis.NewClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)

	authRepository := auth.NewRepository(pool)
	authService := auth.NewService(authRepository, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	usersHandler := users.NewHandler(users.NewRepository(pool))
	chatsRepository := chats.NewRepository(pool)
	chatsService := chats.NewService(chatsRepository)
	chatsHandler := chats.NewHandler(chatsService)

	messagesRepository := messages.NewRepository(pool)
	// Kafka is optional: publishing is skipped when no writer is configured.
	messagesService := messages.NewService(messagesRepository, nil, cfg.KafkaTopic)
	messagesHandler := messages.NewHandler(messagesService)

	uploadHandler := upload.NewHandler()
	wsHub := ws.NewHub(cfg.JWTSecret, redisClient, cfg.AllowedOrigins)
	messagesService.SetBroadcast(wsHub.BroadcastToUsers)

	mux := http.NewServeMux()

	uploadsFS := http.FileServer(http.Dir("./web/uploads"))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", uploadsFS))

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /ws", wsHub.ServeWS)

	authMW := middleware.AuthMiddleware(cfg.JWTSecret)

	mux.Handle("GET /api/v1/users/me", authMW(http.HandlerFunc(usersHandler.GetMe)))
	mux.Handle("GET /api/v1/users/search", authMW(http.HandlerFunc(usersHandler.SearchUsers)))
	mux.Handle("PUT /api/v1/users/profile", authMW(http.HandlerFunc(usersHandler.UpdateProfile)))
	mux.Handle("GET /api/v1/users/{userID}", authMW(http.HandlerFunc(usersHandler.GetUser)))

	mux.Handle("GET /api/v1/chats/", authMW(http.HandlerFunc(chatsHandler.GetMyChats)))
	mux.Handle("POST /api/v1/chats/private", authMW(http.HandlerFunc(chatsHandler.CreatePrivateChat)))
	mux.Handle("POST /api/v1/chats/group", authMW(http.HandlerFunc(chatsHandler.CreateGroupChat)))
	mux.Handle("GET /api/v1/chats/{chatID}", authMW(http.HandlerFunc(chatsHandler.GetChat)))
	mux.Handle("POST /api/v1/chats/{chatID}/members", authMW(http.HandlerFunc(chatsHandler.AddMember)))
	mux.Handle("DELETE /api/v1/chats/{chatID}/members/{userID}", authMW(http.HandlerFunc(chatsHandler.RemoveMember)))

	mux.Handle("GET /api/v1/chats/{chatID}/messages", authMW(http.HandlerFunc(messagesHandler.GetMessages)))
	mux.Handle("POST /api/v1/chats/{chatID}/messages", authMW(http.HandlerFunc(messagesHandler.CreateMessage)))
	mux.Handle("DELETE /api/v1/chats/{chatID}/messages/{messageID}", authMW(http.HandlerFunc(messagesHandler.DeleteMessage)))
	mux.Handle("POST /api/v1/chats/{chatID}/messages/{messageID}/pin", authMW(http.HandlerFunc(messagesHandler.TogglePin)))
	mux.Handle("POST /api/v1/chats/{chatID}/read", authMW(http.HandlerFunc(messagesHandler.MarkAsRead)))

	mux.Handle("POST /api/v1/upload/avatar", authMW(http.HandlerFunc(uploadHandler.UploadAvatar)))

	webFS := http.FileServer(http.Dir("./web"))
	mux.Handle("/", webFS)

	handler := middleware.CORS(mux, cfg.AllowedOrigins)
	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      handler,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		appLogger.Info("HTTP server started", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	appLogger.Info("shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		appLogger.Warn("http shutdown error", zap.Error(err))
	}
}
