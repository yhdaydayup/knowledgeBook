package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"knowledgebook/internal/agent"
	"knowledgebook/internal/api"
	"knowledgebook/internal/config"
	"knowledgebook/internal/conversation"
	"knowledgebook/internal/database"
	"knowledgebook/internal/feishu"
	"knowledgebook/internal/llm"
	"knowledgebook/internal/repository"
	"knowledgebook/internal/service"
)

func main() {
	ctx := context.Background()
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("open postgres: %v", err)
	}
	defer pool.Close()
	redisClient, err := database.OpenRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("open redis: %v", err)
	}
	defer redisClient.Close()
	if cfg.AutoMigrate {
		if err := database.Migrate(ctx, pool, filepath.Join("migrations")); err != nil {
			log.Fatalf("migrate db: %v", err)
		}
	}
	runtimeAgent, err := agent.LoadFromCandidates(filepath.Join("app", "agent"), filepath.Join("/app", "app", "agent"))
	if err != nil {
		log.Fatalf("load runtime agent: %v", err)
	}
	store := repository.New(pool)
	llmClient := llm.NewHTTPClient(cfg)
	messenger := feishu.NewMessenger(cfg.FeishuAppID, cfg.FeishuAppSecret)
	svc := service.New(store, cfg, runtimeAgent, llmClient, messenger)

	// Wire up conversation agent
	if llmClient.Enabled() {
		convHistory := conversation.NewHistory(redisClient,
			cfg.ConvHistoryMaxMessages,
			time.Duration(cfg.ConvHistoryTTLMinutes)*time.Minute,
		)
		toolExecutor := conversation.NewToolExecutor(svc)
		convAgent := conversation.NewAgent(llmClient, toolExecutor, convHistory, runtimeAgent.Prompt("system"))
		svc.ConvAgent = convAgent
		log.Printf("conversation agent enabled (history=%d msgs, ttl=%d min)", cfg.ConvHistoryMaxMessages, cfg.ConvHistoryTTLMinutes)
	}

	handler := api.NewHandler(svc, pool, redisClient)
	server := api.NewServer(handler, cfg.AppPort)
	if cfg.FeishuWSEnabled {
		wsClient := feishu.NewWSClient(cfg.FeishuAppID, cfg.FeishuAppSecret,
			handler.OnWSMessageEvent,
			handler.OnWSCardAction,
		)
		go func() {
			log.Printf("feishu websocket long connection starting...")
			if err := wsClient.Start(context.Background()); err != nil {
				log.Fatalf("feishu websocket client failed: %v", err)
			}
		}()
	}
	log.Printf("app-server listening on :%s", cfg.AppPort)
	server.Spin()
}
