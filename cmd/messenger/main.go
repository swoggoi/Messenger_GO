package main

import (
	"context"
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

	"messenger/internal/core/kafka"
	"messenger/internal/core/logger"
	"messenger/internal/core/redis"
	middleware "messenger/internal/core/transport/http/middleware"
	"messenger/internal/core/transport/ws"
)

func main() {
	_ = godotenv.Load()
	cfg := config.NewConfigMust()

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
	kafkaProducer := kafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaTopic)

	authRepository := auth.NewRepository(pool)
	authService := auth.NewService(authRepository, cfg.JWTSecret)
	authHandler := auth.NewHandler(authService)

	usersHandler := users.NewHandler(pool)
	chatsRepository := chats.NewRepository(pool)
	chatsService := chats.NewService(chatsRepository)
	chatsHandler := chats.NewHandler(chatsService)

	messagesRepository := messages.NewRepository(pool)
	messagesService := messages.NewService(messagesRepository, kafkaProducer, cfg.KafkaTopic)
	messagesHandler := messages.NewHandler(messagesService)

	uploadHandler := upload.NewHandler()
	wsHub := ws.NewHub(cfg.JWTSecret, redisClient)

	mux := http.NewServeMux()

	
	fs := http.FileServer(http.Dir("./web/uploads"))
	mux.Handle("/uploads/", http.StripPrefix("/uploads/", fs))


	mux.HandleFunc("POST /api/v1/auth/register", authHandler.Register)
	mux.HandleFunc("POST /api/v1/auth/login", authHandler.Login)
	mux.HandleFunc("GET /ws", wsHub.ServeWS)

		
	usersMux := http.NewServeMux()
	usersMux.HandleFunc("GET /me", usersHandler.GetMe)
	usersMux.HandleFunc("GET /search", usersHandler.SearchUsers)
	usersMux.HandleFunc("PUT /profile", usersHandler.UpdateProfile)
	mux.Handle("/api/v1/users/", middleware.AuthMiddleware(cfg.JWTSecret)(usersMux))

	
	chatsMux := http.NewServeMux()
	chatsMux.HandleFunc("GET /", chatsHandler.GetMyChats)
	chatsMux.HandleFunc("POST /private", chatsHandler.CreatePrivateChat)
	chatsMux.HandleFunc("POST /group", chatsHandler.CreateGroupChat)
	chatsMux.HandleFunc("GET /{chatID}", chatsHandler.GetChat)
	chatsMux.HandleFunc("GET /{chatID}/messages", messagesHandler.GetMessages)
	chatsMux.HandleFunc("POST /{chatID}/messages", messagesHandler.CreateMessage)
	chatsMux.HandleFunc("POST /{chatID}/read", messagesHandler.MarkAsRead)
	mux.Handle("/api/v1/chats/", middleware.AuthMiddleware(cfg.JWTSecret)(chatsMux))

	
	uploadMux := http.NewServeMux()
	uploadMux.HandleFunc("POST /avatar", uploadHandler.UploadAvatar)
	mux.Handle("/api/v1/upload/", middleware.AuthMiddleware(cfg.JWTSecret)(uploadMux))

	
	handler := middleware.CORS(mux)

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
	server.Shutdown(shutdownCtx)
}
