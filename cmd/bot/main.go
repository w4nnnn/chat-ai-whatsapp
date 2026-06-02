package main

import (
	"context"
	"os"

	"github.com/joho/godotenv"

	"chat-ai-whatsapp/internal/ai"
	"chat-ai-whatsapp/internal/config"
	"chat-ai-whatsapp/internal/handler"
	"chat-ai-whatsapp/internal/logger"
	"chat-ai-whatsapp/internal/search"
	"chat-ai-whatsapp/internal/store"
	"chat-ai-whatsapp/internal/whatsapp"
)

func main() {
	// Load .env file — ignore error kalo gak ada
	_ = godotenv.Load()

	cfg := config.Load()
	logger.Info("Starting WhatsApp AI Chatbot...")

	// Redis store
	redisStore, err := store.New(cfg.RedisURL)
	if err != nil {
		logger.Error("Failed to connect to Redis: %v", err)
		os.Exit(1)
	}
	defer redisStore.Close()
	logger.Info("Redis connected")

	// Search client
	searchClient := search.New(cfg.SearXNGBaseURL)
	logger.Info("SearXNG client ready (%s)", cfg.SearXNGBaseURL)

	// AI client
	aiClient := ai.New(cfg.NineRouterBaseURL, cfg.NineRouterModel, cfg.NineRouterAPIKey)
	logger.Info("AI client ready (%s, model: %s)", cfg.NineRouterBaseURL, cfg.NineRouterModel)

	// Message handler
	msgHandler := handler.New(aiClient, redisStore, searchClient)

	// WhatsApp client
	waClient, err := whatsapp.New(cfg, "whatsmeow-store.db", msgHandler)
	if err != nil {
		logger.Error("Failed to create WhatsApp client: %v", err)
		os.Exit(1)
	}

	if err := waClient.Start(context.Background()); err != nil {
		logger.Error("WhatsApp client error: %v", err)
		os.Exit(1)
	}
}
