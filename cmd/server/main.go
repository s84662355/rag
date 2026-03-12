package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"rag/internal/config"
	"rag/internal/embedding"
	"rag/internal/httpapi"
	"rag/internal/llm"
	"rag/internal/service"
	"rag/internal/store"
	"rag/internal/vector"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		panic(err)
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfgPath := os.Getenv("RAG_CONFIG")
	if cfgPath == "" {
		cfgPath = "config.yaml"
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		logger.Fatal("load config failed", zap.String("config_path", cfgPath), zap.Error(err))
	}

	mysqlStore, err := store.NewMySQLStore(cfg.MySQL.DSN)
	if err != nil {
		logger.Fatal("init mysql failed", zap.Error(err))
	}
	defer mysqlStore.Close()

	esClient := vector.NewESClient(cfg.Vector.ES.Address, cfg.Vector.IndexName, cfg.Vector.Dims)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	if err := esClient.EnsureIndex(ctx); err != nil {
		cancel()
		logger.Fatal("ensure es index failed", zap.Error(err))
	}
	cancel()

	svc := service.NewRAGService(
		mysqlStore,
		esClient,
		embedding.NewClient(cfg.Embedding.BaseURL, cfg.Embedding.Model),
		llm.NewClient(cfg.Rewrite.APIKey, cfg.Rewrite.BaseURL, cfg.Rewrite.Model),
		llm.NewClient(cfg.Rerank.APIKey, cfg.Rerank.BaseURL, cfg.Rerank.Model),
		llm.NewClient(cfg.QA.APIKey, cfg.QA.BaseURL, cfg.QA.Model),
		llm.NewClient(cfg.Chat.APIKey, cfg.Chat.BaseURL, cfg.Chat.Model),
		cfg.Chunk.MaxChars,
		cfg.Chunk.Overlap,
	)

	handler := httpapi.NewHandler(svc, cfg.Server.StaticDir, logger)
	srv := &http.Server{
		Addr:              cfg.Server.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 15 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      120 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("RAG server listening", zap.String("addr", cfg.Server.Addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	received := <-sig
	logger.Info("shutdown signal received", zap.String("signal", received.String()))

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("graceful shutdown failed", zap.Error(err))
	}
}
