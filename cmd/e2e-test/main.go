package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"knowledgebook/internal/agent"
	"knowledgebook/internal/config"
	"knowledgebook/internal/conversation"
	"knowledgebook/internal/database"
	"knowledgebook/internal/feishu"
	"knowledgebook/internal/llm"
	"knowledgebook/internal/repository"
	"knowledgebook/internal/service"
)

// E2E test tool — connects to real PostgreSQL + Redis + LLM, simulates
// multi-turn conversations through the full service stack.
//
// Each scenario uses a unique chatID so conversation histories don't leak.
// Run from project root:
//   GOFLAGS="-tags=stdjson" go run ./cmd/e2e-test/

const (
	testOpenID   = "e2e_test_user_001"
	testUserName = "E2E测试用户"
)

var scenarioCounter int

func nextChatID() string {
	scenarioCounter++
	return fmt.Sprintf("e2e_chat_%03d", scenarioCounter)
}

type testCase struct {
	name    string
	message string
	checks  []checkFunc
}

type checkFunc func(result *service.BotCommandResult, svc *service.Services) error

func main() {
	ctx := context.Background()
	svc, cleanup := setupServices(ctx)
	defer cleanup()

	passed, failed, total := 0, 0, 0

	type scenario struct {
		name  string
		tests []testCase
	}

	scenarios := []scenario{
		{
			name: "场景A: 闲聊不创建草稿",
			tests: []testCase{
				{"A1: 打招呼", "你好", []checkFunc{expectNoCard, expectReplyNotEmpty, expectIntentAgent}},
				{"A2: 陈述性内容（无记录意图）", "飞书接口签名验证失败是因为 header token 不匹配", []checkFunc{expectNoCard, expectReplyNotEmpty}},
			},
		},
		{
			name: "场景B: 创建 → 确认保存",
			tests: []testCase{
				{"B1: 明确记录", "帮我记一下，飞书接口限流策略改为令牌桶算法，每秒100个请求", []checkFunc{expectHasCard, expectReplyNotEmpty, expectDraftInDB}},
				{"B2: 确认保存", "确认保存", []checkFunc{expectReplyNotEmpty}},
			},
		},
		{
			name: "场景C: 创建 → 拒绝保存",
			tests: []testCase{
				{"C1: 明确记录", "帮我记一下，Redis 集群节点间通信用 gossip 协议", []checkFunc{expectHasCard, expectDraftInDB}},
				{"C2: 拒绝保存", "算了，不要了", []checkFunc{expectNoCard, expectReplyNotEmpty}},
			},
		},
		{
			name: "场景D: 创建 → 修改分类",
			tests: []testCase{
				{"D1: 创建草稿", "帮我记一下，Nginx 反向代理配置 proxy_pass 末尾要加斜杠", []checkFunc{expectHasCard}},
				{"D2: 修改分类", "分类改到 运维/Nginx配置", []checkFunc{expectReplyNotEmpty}},
			},
		},
		{
			name: "场景E: 创建 → 修订内容（补充日期）",
			tests: []testCase{
				{"E1: 创建草稿", "帮我记录一下，PostgreSQL 分区表在数据量超过 1000 万行时性能优势明显", []checkFunc{expectHasCard}},
				{"E2: 补充日期", "帮我补充上今天的日期", []checkFunc{expectHasCard, expectReplyNotEmpty}},
			},
		},
		{
			name: "场景F: 创建 → 修改标题",
			tests: []testCase{
				{"F1: 创建草稿", "帮我记一下，Go 的 context 取消传播是通过 channel close 实现的", []checkFunc{expectHasCard}},
				{"F2: 修改标题", "标题改成「Go context 取消机制原理」", []checkFunc{expectReplyNotEmpty}},
			},
		},
		{
			name: "场景G: 搜索知识",
			tests: []testCase{
				{"G1: 搜索请求", "之前有没有记录过关于接口限流的内容？", []checkFunc{expectNoCard, expectReplyNotEmpty}},
			},
		},
		{
			name: "场景H: 时间感知",
			tests: []testCase{
				{"H1: 询问日期", "今天几号？", []checkFunc{expectReplyContains(time.Now().Format("2006"))}},
			},
		},
		{
			name: "场景I: 模糊输入不创建草稿",
			tests: []testCase{
				{"I1: 模糊肯定（无记录意图）", "这个信息挺有用的", []checkFunc{expectNoCard, expectReplyNotEmpty}},
			},
		},
		{
			name: "场景J: 连续创建后确认应询问",
			tests: []testCase{
				{"J1: 创建第一条", "帮我记一下，Docker 多阶段构建可以大幅缩小镜像体积", []checkFunc{expectHasCard}},
				{"J2: 创建第二条", "再帮我记一下，Kubernetes Pod 的 liveness probe 建议设置合理的超时时间", []checkFunc{expectHasCard}},
				{"J3: 确认保存（未指定哪条）", "确认保存", []checkFunc{expectReplyNotEmpty}},
			},
		},
		{
			name: "场景K: guard 拦截（技术陈述无记录意图）",
			tests: []testCase{
				{"K1: 技术陈述", "Redis 集群用 gossip 协议进行节点间通信", []checkFunc{expectNoCard, expectReplyNotEmpty}},
			},
		},
	}

	for _, s := range scenarios {
		chatID := nextChatID() // fresh history per scenario

		fmt.Printf("\n══════════════════════════════════════════════════════════\n")
		fmt.Printf("  %s  [chatID=%s]\n", s.name, chatID)
		fmt.Printf("══════════════════════════════════════════════════════════\n")

		for _, tc := range s.tests {
			total++
			fmt.Printf("\n  ▸ %s\n", tc.name)
			fmt.Printf("    用户: %s\n", tc.message)

			result, err := svc.ExecuteBotMessageWithContext(ctx, testOpenID, testUserName, tc.message, map[string]any{
				"chat_id": chatID,
			})
			if err != nil {
				fmt.Printf("    ✗ 执行失败: %v\n", err)
				failed++
				continue
			}

			fmt.Printf("    回复: %s\n", truncate(result.Reply, 120))
			if result.CardMarkdown != "" {
				fmt.Printf("    卡片: [有] %s\n", truncate(result.CardMarkdown, 80))
			}
			if result.Data != nil {
				if dataJSON, err := json.Marshal(result.Data); err == nil && len(dataJSON) > 2 {
					fmt.Printf("    数据: %s\n", truncate(string(dataJSON), 150))
				}
			}

			allOK := true
			for _, check := range tc.checks {
				if err := check(result, svc); err != nil {
					fmt.Printf("    ✗ 检查失败: %v\n", err)
					allOK = false
				}
			}
			if allOK {
				fmt.Printf("    ✓ 通过\n")
				passed++
			} else {
				failed++
			}
		}
	}

	fmt.Printf("\n══════════════════════════════════════════════════════════\n")
	fmt.Printf("  测试结果: %d 通过, %d 失败, 共 %d 个\n", passed, failed, total)
	fmt.Printf("══════════════════════════════════════════════════════════\n\n")

	if failed > 0 {
		os.Exit(1)
	}
}

// ── Setup ────────────────────────────────────────────────────────────────────

func setupServices(ctx context.Context) (*service.Services, func()) {
	setDefault("POSTGRES_DSN", "postgres://knowledgebook:change_me@127.0.0.1:5432/knowledgebook?sslmode=disable")
	setDefault("REDIS_ADDR", "127.0.0.1:6379")
	setDefault("LLM_ENABLED", "true")
	setDefault("LLM_BASE_URL", "https://dashscope.aliyuncs.com/compatible-mode/v1")
	setDefault("LLM_API_KEY", "sk-fefe18569b4047989d428fbba4e2d001")
	setDefault("LLM_MODEL", "qwen3-max")
	setDefault("LLM_TIMEOUT_MS", "20000")
	setDefault("LLM_MAX_TOKENS", "2000")
	setDefault("CONV_HISTORY_MAX_MESSAGES", "20")
	setDefault("CONV_HISTORY_TTL_MINUTES", "120")
	setDefault("AUTO_MIGRATE", "false")
	setDefault("APP_PORT", "9999")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load config: %v", err)
	}
	pool, err := database.Open(ctx, cfg.PostgresDSN)
	if err != nil {
		log.Fatalf("connect postgres: %v", err)
	}
	redisClient, err := database.OpenRedis(ctx, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("connect redis: %v", err)
	}
	runtimeAgent, err := agent.LoadFromCandidates(filepath.Join("app", "agent"))
	if err != nil {
		log.Fatalf("load runtime agent: %v", err)
	}

	store := repository.New(pool)
	llmClient := llm.NewHTTPClient(cfg)
	messenger := feishu.NewMessenger("", "") // disabled for tests
	svc := service.New(store, cfg, runtimeAgent, llmClient, messenger)

	convHistory := conversation.NewHistory(redisClient,
		cfg.ConvHistoryMaxMessages,
		time.Duration(cfg.ConvHistoryTTLMinutes)*time.Minute,
	)
	toolExecutor := conversation.NewToolExecutor(svc)
	convAgent := conversation.NewAgent(llmClient, toolExecutor, convHistory, runtimeAgent.Prompt("system"))
	svc.ConvAgent = convAgent

	fmt.Println("✓ PostgreSQL, Redis, LLM 连接成功，Agent 已初始化")
	return svc, func() { pool.Close(); redisClient.Close() }
}

// ── Checks ───────────────────────────────────────────────────────────────────

func expectNoCard(result *service.BotCommandResult, _ *service.Services) error {
	if strings.TrimSpace(result.CardMarkdown) != "" {
		return fmt.Errorf("不应有卡片，但返回了卡片")
	}
	return nil
}

func expectHasCard(result *service.BotCommandResult, _ *service.Services) error {
	if strings.TrimSpace(result.CardMarkdown) == "" {
		return fmt.Errorf("应有卡片，但无卡片返回")
	}
	return nil
}

func expectReplyNotEmpty(result *service.BotCommandResult, _ *service.Services) error {
	if strings.TrimSpace(result.Reply) == "" {
		return fmt.Errorf("回复为空")
	}
	return nil
}

func expectIntentAgent(result *service.BotCommandResult, _ *service.Services) error {
	if result.Command != "agent" {
		return fmt.Errorf("预期 intent=agent, 实际=%s (可能走了 V2 fallback)", result.Command)
	}
	return nil
}

func expectReplyContains(keywords ...string) checkFunc {
	return func(result *service.BotCommandResult, _ *service.Services) error {
		for _, kw := range keywords {
			if !strings.Contains(result.Reply, kw) {
				return fmt.Errorf("回复应包含 %q", kw)
			}
		}
		return nil
	}
}

func expectDraftInDB(result *service.BotCommandResult, _ *service.Services) error {
	data, ok := result.Data.(map[string]any)
	if !ok {
		return fmt.Errorf("Data 不是 map[string]any: %T", result.Data)
	}
	draftMap, ok := data["draft"].(map[string]any)
	if !ok {
		return fmt.Errorf("Data 中没有 draft map (draft_id 可能丢失)")
	}
	id, _ := draftMap["id"].(float64)
	if id == 0 {
		return fmt.Errorf("draft id = 0")
	}
	fmt.Printf("    [DB] 草稿 #%d 已创建\n", int64(id))
	return nil
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func truncate(s string, max int) string {
	s = strings.ReplaceAll(strings.TrimSpace(s), "\n", " | ")
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "..."
}

func setDefault(key, val string) {
	if os.Getenv(key) == "" {
		os.Setenv(key, val)
	}
}
