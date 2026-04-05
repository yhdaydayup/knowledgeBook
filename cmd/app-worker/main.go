package main

import (
	"context"
	"log"
	"path/filepath"
	"time"

	"knowledgebook/internal/agent"
	"knowledgebook/internal/config"
	"knowledgebook/internal/database"
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
	svc := service.New(store, cfg, runtimeAgent, llmClient)
	log.Printf("app-worker started")
	ticker := time.NewTicker(service.SleepUntilNextTick())
	defer ticker.Stop()
	for {
		if err := svc.ProcessTasks(ctx); err != nil {
			log.Printf("process tasks error: %v", err)
		}
		<-ticker.C
	}
}
