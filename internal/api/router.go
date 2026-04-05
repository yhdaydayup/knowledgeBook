package api

import (
	stdctx "context"

	"github.com/cloudwego/hertz/pkg/app/server"
)

func NewServer(handler *Handler, port string) *server.Hertz {
	h := server.Default(server.WithHostPorts("0.0.0.0:" + port))
	h.GET("/healthz", handler.Healthz)
	h.GET("/readyz", handler.Readyz)
	h.POST("/api/v1/feishu/events", handler.FeishuEvents)
	h.GET("/api/v1/feishu/oauth/callback", handler.FeishuOAuthCallback)
	h.POST("/api/v1/feishu/card-callback", handler.FeishuCardCallback)
	h.GET("/api/v1/bot/command", handler.HandleBotCommand)
	h.POST("/mcp", handler.HandleMCP)

	h.POST("/api/v1/knowledge", handler.CreateKnowledge)
	h.GET("/api/v1/knowledge/search", handler.SearchKnowledge)
	h.GET("/api/v1/knowledge/:id", handler.GetKnowledge)
	h.PUT("/api/v1/knowledge/:id", handler.UpdateKnowledge)
	h.POST("/api/v1/knowledge/:id/move-category", handler.MoveKnowledge)
	h.DELETE("/api/v1/knowledge/:id", handler.SoftDeleteKnowledge)
	h.POST("/api/v1/knowledge/:id/restore", handler.RestoreKnowledge)
	h.POST("/api/v1/knowledge/:id/sync-from-doc", handler.SyncFromDoc)

	h.POST("/api/v1/drafts/:id/approve", handler.ApproveDraft)
	h.POST("/api/v1/drafts/:id/ignore", handler.IgnoreDraft)
	h.POST("/api/v1/drafts/:id/later", handler.LaterDraft)
	h.GET("/api/v1/drafts/later", handler.ListLaterDrafts)

	h.GET("/api/v1/categories/tree", handler.ListCategories)
	h.POST("/api/v1/categories", handler.CreateCategory)

	h.POST("/api/v1/doc-sync/knowledge/:id", handler.TriggerKnowledgeSync)
	h.GET("/api/v1/doc-sync/task/:id", handler.GetTask)

	_ = stdctx.Background()
	return h
}
