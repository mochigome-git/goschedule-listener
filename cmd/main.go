package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"goschedule-listener/internal/config"
	"goschedule-listener/internal/db"
	"goschedule-listener/internal/listener"
	"goschedule-listener/internal/mqtt"

	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.Load()

	dbClient, err := db.New(cfg)
	if err != nil {
		log.Fatal("DB init failed:", err)
	}
	defer dbClient.Close()

	mqClient, err := mqtt.New(cfg)
	if err != nil {
		log.Fatal("MQTT init failed:", err)
	}
	defer mqClient.Disconnect()

	// Graceful shutdown on SIGTERM/SIGINT (ECS sends SIGTERM on task stop)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	l := listener.New(cfg, dbClient, mqClient, logger)

	if err := l.Start(ctx); err != nil {
		log.Fatal("Listener error:", err)
	}
}
