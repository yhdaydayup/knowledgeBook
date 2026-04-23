package conversation

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"testing"
	"time"

	"knowledgebook/internal/config"
	"knowledgebook/internal/llm"
)

// ════════════════════════════════════════════════════════════════════════════════
// LLM integration tests with quantitative evaluation framework.
//
// Run with: GOFLAGS="-tags=stdjson" go test -v -run TestLLMEval -timeout 300s ./internal/conversation/
// Quick (N=1, backward compat): GOFLAGS="-tags=stdjson" go test -v -run TestLLM_ -timeout 120s ./internal/conversation/
//
// Requirements: LLM_BASE_URL, LLM_API_KEY, LLM_MODEL env vars must be set.
// ════════════════════════════════════════════════════════════════════════════════

const testSystemPrompt = `# knowledgeBook 知识沉淀助手

你是用户的私人知识沉淀助手，帮助用户通过自然对话管理个人知识库。

## 你的性格

- 像一个可靠、随和的朋友在帮忙整理笔记
- 简洁自然，不啰嗦，不说套话
- 理解上下文，能承接上一轮对话继续
- 遇到不确定的情况会坦诚说明，而不是瞎猜

## 你能做什么

你可以调用以下工具帮助用户：

- **create_knowledge_draft**：用户想记录一个结论、经验、方案、规则时，帮他提炼整理成草稿。草稿需要用户确认后才正式保存。
- **confirm_knowledge_draft**：用户说"保存""确认"时，确认保存之前创建的草稿。
- **reject_knowledge_draft**：用户说"不要了""丢弃""算了"时，丢弃草稿。
- **update_draft_category**：用户想修改草稿的分类路径时使用。
- **search_knowledge**：用户想查找之前记录的知识时使用。
- **check_similarity**：用户想知道某条内容是否和已有知识重复或冲突时使用。
- **list_pending_drafts**：当需要确认或操作草稿但不确定是哪一条时，先列出待确认的草稿。
- **revise_knowledge_draft**：用户想修改、补充、完善刚创建的草稿内容时使用。

## 何时使用工具

- 用户消息中**包含**"帮我记""记一下""记录下来""保存这个"等记录指令词 → 用 create_knowledge_draft
- 用户只是在聊天、讨论、描述情况、陈述事实、分享经验或观点，但**没有**说要记录 → **绝对不要**调 create_knowledge_draft，正常对话回复即可。如果你觉得内容值得记录，可以建议"要不要帮你记下来？"，但不要直接创建草稿
- 用户问"之前怎么定的""有没有记录""帮我找" → 用 search_knowledge
- 用户说"确认""保存""就这样" → 用 confirm_knowledge_draft
- 用户说"不要""丢弃""算了" → 用 reject_knowledge_draft
- 用户说"改到某个分类" → 用 update_draft_category
- 用户问"是不是重复""有没有冲突" → 用 check_similarity
- 用户要求修改、补充、完善刚创建的草稿内容 → 用 revise_knowledge_draft（不要用 create_knowledge_draft 重新创建）
- 用户只是闲聊、打招呼、说谢谢 → 直接用自然语言回复，不需要调工具

## 回复风格

- 用自然中文回复，像朋友聊天
- 创建草稿后，简要告知内容、分类，并提示用户可以确认保存、拒绝或修改分类
- 确认/拒绝后，简短确认即可
- 搜索结果用自然语言总结，附上关键证据
- 不要输出 JSON、代码块或命令格式
- 不要重复列出"支持的指令列表"

## 重要规则

- **不要擅自创建草稿！** 这是最重要的规则。调用 create_knowledge_draft 的**唯一前提**是：用户的消息中包含明确的记录指令词，比如"帮我记""记一下""记录下来""保存这个""帮我记录"。如果用户的消息中**没有**这些词，就**绝对不要**调用 create_knowledge_draft。
- **以下情况不能创建草稿（即使内容看起来很有价值）：**
  - 用户只是陈述一个事实或经验："Nginx proxy_pass 末尾要加斜杠" → 不创建，正常回复讨论
  - 用户在分享技术信息："飞书验签失败是因为 header token 不匹配" → 不创建，正常回复讨论
  - 用户在讨论工作进展："开始 V3 版本的开发和测试工作" → 不创建，正常回复
  - 用户的消息看起来像知识点但没说要记录 → 不创建，可以问"要不要帮你记下来？"
- **以下情况才能创建草稿：**
  - "帮我记一下，Nginx proxy_pass 末尾要加斜杠" → 有"帮我记一下"，创建
  - "记录下来：飞书验签失败原因是 token 不匹配" → 有"记录下来"，创建
  - "把这个保存一下" → 有"保存一下"，创建
- 正式写入知识库必须经过用户确认，你不能自动保存
- 如果当前有多条待确认草稿且用户没有指定是哪条，先用 list_pending_drafts 列出，然后询问用户
- 如果用户说"确认保存"但当前没有待确认草稿，如实告知
- 遇到不明确的请求，优先澄清而非猜测执行
- 不要编造知识库中不存在的内容`

// ── Infrastructure ──────────────────────────────────────────────────────────

func skipIfNoLLM(t *testing.T) llm.Client {
	t.Helper()
	baseURL := os.Getenv("LLM_BASE_URL")
	apiKey := os.Getenv("LLM_API_KEY")
	model := os.Getenv("LLM_MODEL")
	if baseURL == "" || apiKey == "" || model == "" {
		t.Skip("LLM env vars not set (LLM_BASE_URL, LLM_API_KEY, LLM_MODEL), skipping integration test")
	}
	client := llm.NewHTTPClient(config.Config{
		LLMEnabled:   true,
		LLMBaseURL:   baseURL,
		LLMAPIKey:    apiKey,
		LLMModel:     model,
		LLMTimeoutMS: 20000,
		LLMMaxTokens: 2000,
	})
	if !client.Enabled() {
		t.Skip("LLM client not enabled")
	}
	return client
}

func envIntOrDefault(key string, def int) int {
	s := os.Getenv(key)
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

// chatOnce sends a single user message to the LLM with tools and returns the response.
func chatOnce(t *testing.T, client llm.Client, userMessage string) *llm.ChatResponse {
	t.Helper()
	now := time.Now()
	systemContent := testSystemPrompt + fmt.Sprintf("\n\n当前时间：%s", now.Format("2006年01月02日 15:04 (Monday)"))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Messages: []llm.ChatMessage{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: userMessage},
		},
		Tools:       AgentTools(),
		Temperature: 0.3,
		MaxTokens:   2000,
	})
	if err != nil {
		t.Fatalf("LLM Chat failed: %v", err)
	}
	return resp
}

// buildDraftContext builds a message history simulating:
//  1. User asked to record something
//  2. Assistant called create_knowledge_draft
//  3. Tool returned draft result (ID=85)
//  4. Assistant replied confirming the draft
func buildDraftContext() []llm.ChatMessage {
	return []llm.ChatMessage{
		{
			Role:    "user",
			Content: "帮我记一下，飞书接口签名验证失败是因为 header 里的 token 和配置不一致",
		},
		{
			Role:    "assistant",
			Content: "",
			ToolCalls: []llm.ToolCall{
				{
					ID:   "call_001",
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "create_knowledge_draft",
						Arguments: `{"title":"飞书接口签名验证失败排查","content":"飞书接口签名验证失败是因为 header 里的 token 和配置不一致","categoryPath":"工作/默认项目"}`,
					},
				},
			},
		},
		{
			Role:       "tool",
			Content:    `{"id":85,"title":"飞书接口签名验证失败排查","normalized_summary":"签名验证失败原因为 header token 与配置不一致","normalized_points":["header token 不匹配","需要检查配置一致性"],"recommended_category_path":"工作/默认项目/接口设计","status":"PENDING"}`,
			ToolCallID: "call_001",
		},
		{
			Role:    "assistant",
			Content: "已经帮你整理好了一条草稿：\n\n- 标题：飞书接口签名验证失败排查\n- 分类：工作/默认项目/接口设计\n- 摘要：签名验证失败原因为 header token 与配置不一致\n\n你可以确认保存，也可以修改内容或分类，或者不要了也行。",
		},
	}
}

// chatMultiTurn sends a follow-up user message with draft context history.
func chatMultiTurn(t *testing.T, client llm.Client, history []llm.ChatMessage, userMessage string) *llm.ChatResponse {
	t.Helper()
	now := time.Now()
	systemContent := testSystemPrompt + fmt.Sprintf("\n\n当前时间：%s", now.Format("2006年01月02日 15:04 (Monday)"))

	messages := make([]llm.ChatMessage, 0, len(history)+2)
	messages = append(messages, llm.ChatMessage{Role: "system", Content: systemContent})
	messages = append(messages, history...)
	messages = append(messages, llm.ChatMessage{Role: "user", Content: userMessage})

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := client.Chat(ctx, llm.ChatRequest{
		Messages:    messages,
		Tools:       AgentTools(),
		Temperature: 0.3,
		MaxTokens:   2000,
	})
	if err != nil {
		t.Fatalf("LLM Chat failed: %v", err)
	}
	return resp
}

func toolNames(calls []llm.ToolCall) []string {
	names := make([]string, len(calls))
	for i, c := range calls {
		names[i] = c.Function.Name
	}
	return names
}

// ── Scenario Definitions ────────────────────────────────────────────────────

// allScenarios returns the complete set of 22 evaluation scenarios (13 single-turn + 9 multi-turn).
func allScenarios() []ScenarioSpec {
	draftCtx := buildDraftContext()
	return []ScenarioSpec{
		// ── Single-turn: quality ──────────────────────────────────────
		{
			Name:     "S01_打招呼",
			Category: "quality",
			UserMsg:  "你好",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckResponseQuality(),
			},
		},
		{
			Name:     "S02_感谢",
			Category: "quality",
			UserMsg:  "谢谢你",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckResponseQuality(),
			},
		},
		{
			Name:     "S03_闲聊",
			Category: "quality",
			UserMsg:  "今天天气真好啊",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckResponseQuality(),
			},
		},
		// ── Single-turn: safety_critical ──────────────────────────────
		{
			Name:     "S04_陈述无意图",
			Category: "safety_critical",
			UserMsg:  "飞书验签失败是因为 header token 不匹配",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckToolNotCalled("create_knowledge_draft"),
				CheckResponseQuality(),
			},
		},
		{
			Name:     "S05_工作讨论无意图",
			Category: "safety_critical",
			UserMsg:  "开始 V3 版本的开发和测试工作。",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckToolNotCalled("create_knowledge_draft"),
				CheckResponseQuality(),
			},
		},
		{
			Name:     "S06_技术陈述无意图",
			Category: "safety_critical",
			UserMsg:  "Go 的 goroutine 调度器用的是 GMP 模型",
			Checks: []CheckFunc{
				CheckToolNotCalled("create_knowledge_draft"),
				CheckResponseQuality(),
			},
		},
		// ── Single-turn: core ─────────────────────────────────────────
		{
			Name:     "S07_明确记录_帮我记",
			Category: "core",
			UserMsg:  "帮我记一下，飞书验签失败是因为 header token 不匹配",
			Checks: []CheckFunc{
				CheckToolExpected("create_knowledge_draft"),
				CheckArgsPresent("create_knowledge_draft", "content"),
				CheckArgContains("create_knowledge_draft", "content", "token", "不匹配"),
			},
		},
		{
			Name:     "S08_明确记录_记录下来",
			Category: "core",
			UserMsg:  "把这个记录下来：接口限流策略改为令牌桶算法",
			Checks: []CheckFunc{
				CheckToolExpected("create_knowledge_draft"),
				CheckArgsPresent("create_knowledge_draft", "content"),
				CheckArgContains("create_knowledge_draft", "content", "令牌桶"),
			},
		},
		{
			Name:     "S09_搜索_结论查询",
			Category: "core",
			UserMsg:  "之前关于接口限流的结论是什么？",
			Checks: []CheckFunc{
				CheckToolExpected("search_knowledge"),
				CheckArgsPresent("search_knowledge", "query"),
			},
		},
		{
			Name:     "S10_搜索_有没有记录",
			Category: "core",
			UserMsg:  "有没有记录过飞书 API 的签名验证方式？",
			Checks: []CheckFunc{
				CheckToolExpected("search_knowledge"),
			},
		},
		// ── Single-turn: quality (continued) ─────────────────────────
		{
			Name:     "S11_时间感知",
			Category: "quality",
			UserMsg:  "今天是几号？",
			Checks: []CheckFunc{
				CheckToolExpected(""),
				CheckResponseContains(time.Now().Format("2006")),
			},
		},
		// ── Single-turn: core (continued) ────────────────────────────
		{
			Name:     "S12_相似度检查",
			Category: "core",
			UserMsg:  "这条内容和之前的有没有重复？帮我检查一下：飞书接口签名失败",
			Checks: []CheckFunc{
				CheckToolExpected("check_similarity"),
			},
		},
		// ── Single-turn: quality (continued) ─────────────────────────
		{
			Name:     "S13_模糊输入",
			Category: "quality",
			UserMsg:  "这个信息挺有用的",
			Checks: []CheckFunc{
				CheckToolNotCalled("create_knowledge_draft"),
				CheckResponseQuality(),
			},
		},
		// ── Multi-turn: core (with draft context, ID=85) ─────────────
		{
			Name:     "M01_确认保存",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "确认保存",
			Checks: []CheckFunc{
				CheckToolExpected("confirm_knowledge_draft"),
				CheckDraftIDCorrect("confirm_knowledge_draft", 85),
			},
		},
		{
			Name:     "M02_确认口语化",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "可以，保存吧",
			Checks: []CheckFunc{
				CheckToolExpected("confirm_knowledge_draft"),
			},
		},
		{
			Name:     "M03_拒绝_不要了",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "算了，不要了",
			Checks: []CheckFunc{
				CheckToolExpected("reject_knowledge_draft"),
			},
		},
		{
			Name:     "M04_拒绝_丢弃",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "丢弃吧",
			Checks: []CheckFunc{
				CheckToolExpected("reject_knowledge_draft"),
			},
		},
		{
			Name:     "M05_修改分类",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "分类改到 工作/飞书集成/签名验证",
			Checks: []CheckFunc{
				CheckToolExpected("update_draft_category"),
				CheckArgsPresent("update_draft_category", "categoryPath"),
				CheckArgContains("update_draft_category", "categoryPath", "飞书"),
			},
		},
		{
			Name:     "M06_修订_补充日期",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "帮我补充上今天的日期",
			Checks: []CheckFunc{
				CheckToolExpected("revise_knowledge_draft"),
				CheckToolNotCalled("create_knowledge_draft"),
			},
		},
		{
			Name:     "M07_修订_改标题",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "标题改成「飞书签名失败的根因」",
			Checks: []CheckFunc{
				CheckToolExpected("revise_knowledge_draft"),
				CheckToolNotCalled("create_knowledge_draft"),
			},
		},
		{
			Name:     "M08_修订_追加内容",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "内容再加上：解决方案是重新生成 token",
			Checks: []CheckFunc{
				CheckToolExpected("revise_knowledge_draft"),
				CheckArgsPresent("revise_knowledge_draft", "content"),
				CheckArgContains("revise_knowledge_draft", "content", "token"),
			},
		},
		{
			Name:     "M09_指定ID确认",
			Category: "core",
			History:  draftCtx,
			UserMsg:  "确认保存草稿 85",
			Checks: []CheckFunc{
				CheckToolExpected("confirm_knowledge_draft"),
				CheckDraftIDCorrect("confirm_knowledge_draft", 85),
			},
		},
	}
}

// ── Eval Runner ─────────────────────────────────────────────────────────────

// evalScenario runs a single scenario N times and returns the aggregated result.
func evalScenario(t *testing.T, client llm.Client, spec ScenarioSpec, nRuns int) ScenarioResult {
	t.Helper()
	runs := make([]EvalRunResult, 0, nRuns)

	for i := 0; i < nRuns; i++ {
		start := time.Now()

		var resp *llm.ChatResponse
		if len(spec.History) > 0 {
			resp = chatMultiTurn(t, client, spec.History, spec.UserMsg)
		} else {
			resp = chatOnce(t, client, spec.UserMsg)
		}

		// Run all checks
		scores := make([]DimensionScore, 0, len(spec.Checks))
		for _, check := range spec.Checks {
			scores = append(scores, check(&spec, resp))
		}

		composite := ComputeComposite(scores)
		pass := composite >= PassCompositeThreshold

		runs = append(runs, EvalRunResult{
			RunIndex:   i,
			Response:   resp,
			Scores:     scores,
			Composite:  composite,
			Pass:       pass,
			DurationMS: time.Since(start).Milliseconds(),
		})
	}

	return ComputeScenarioResult(spec.Name, spec.Category, runs)
}

// printRunDetail logs per-run dimension scores.
func printRunDetail(t *testing.T, run EvalRunResult) {
	t.Helper()
	for _, ds := range run.Scores {
		mark := "✓"
		if ds.Score < ds.MaxScore*0.6 {
			mark = "✗"
		}
		t.Logf("      %s %-22s %.1f/%.1f  %s", mark, ds.Dimension, ds.Score, ds.MaxScore, ds.Notes)
	}
	pass := "PASS"
	if !run.Pass {
		pass = "FAIL"
	}
	t.Logf("      → composite=%.3f %s (%dms)", run.Composite, pass, run.DurationMS)
}

// printScenarioSummary logs the aggregated scenario result.
func printScenarioSummary(t *testing.T, sr ScenarioResult) {
	t.Helper()
	threshold := CategoryThreshold[sr.Category]
	met := "✓"
	if !sr.MeetsThreshold {
		met = "✗"
	}
	t.Logf("  %s %-30s passRate=%.0f%% (>= %.0f%%)  mean=%.3f stddev=%.3f",
		met, sr.ScenarioName, sr.PassRate*100, threshold*100, sr.MeanComposite, sr.StdDevComposite)
}

// printEvalReport prints the final evaluation report.
func printEvalReport(t *testing.T, suite EvalSuiteResult) {
	t.Helper()
	t.Logf("")
	t.Logf("═══════════════════════════════════════════════════")
	t.Logf("  LLM 评估报告  (N=%d runs per scenario)", envIntOrDefault("EVAL_N_RUNS", DefaultNRuns))
	t.Logf("═══════════════════════════════════════════════════")
	t.Logf("")
	t.Logf("  场景总数: %d      通过: %d      失败: %d",
		suite.TotalScenarios, suite.PassedScenarios, suite.FailedScenarios)
	t.Logf("")
	t.Logf("  按类别:")
	for _, cat := range []string{"safety_critical", "core", "quality"} {
		cs, ok := suite.ByCategory[cat]
		if !ok {
			continue
		}
		met := "✓"
		if !cs.Met {
			met = "✗"
		}
		t.Logf("    %-20s %d/%d (%.1f%%)  %s", cat+":", cs.Passed, cs.Total, cs.PassRate*100, met)
	}
	t.Logf("")
	t.Logf("  按维度平均分 (0-5):")
	for _, dim := range []ScoreDimension{DimToolSelection, DimArgumentQuality, DimResponseQuality, DimSafetyCompliance, DimContextUtilization} {
		if avg, ok := suite.ByDimension[dim]; ok {
			t.Logf("    %-24s %.1f", dim+":", avg)
		}
	}
	t.Logf("")
	t.Logf("  综合得分: %.2f / 1.00", suite.OverallScore)
	t.Logf("═══════════════════════════════════════════════════")
}

// ── Main Eval Entry Point ───────────────────────────────────────────────────

func TestLLMEval(t *testing.T) {
	client := skipIfNoLLM(t)
	nRuns := envIntOrDefault("EVAL_N_RUNS", DefaultNRuns)
	t.Logf("Running %d scenarios × %d runs each", len(allScenarios()), nRuns)

	allResults := make([]ScenarioResult, 0, len(allScenarios()))

	for _, spec := range allScenarios() {
		spec := spec // capture
		t.Run(spec.Name, func(t *testing.T) {
			result := evalScenario(t, client, spec, nRuns)

			// Print per-run details
			for _, run := range result.Runs {
				t.Logf("    Run %d:", run.RunIndex)
				printRunDetail(t, run)
			}

			// Print scenario summary
			printScenarioSummary(t, result)

			if !result.MeetsThreshold {
				t.Errorf("FAIL: %s passRate=%.0f%% < threshold %.0f%% for category %q",
					spec.Name, result.PassRate*100, CategoryThreshold[spec.Category]*100, spec.Category)
			}

			allResults = append(allResults, result)
		})
	}

	// Print overall report after all subtests
	t.Run("_Report", func(t *testing.T) {
		suite := ComputeSuiteResult(allResults)
		printEvalReport(t, suite)

		// Marshal to JSON for CI/CD integration
		reportJSON, err := json.MarshalIndent(suite, "", "  ")
		if err == nil {
			t.Logf("\nJSON Report:\n%s", string(reportJSON))
		}
	})
}

// ── Backward-Compatible Thin Wrappers ───────────────────────────────────────
// These run each scenario once (N=1) for quick smoke-testing under the old names.

func runSingleScenario(t *testing.T, client llm.Client, spec ScenarioSpec) {
	t.Helper()
	result := evalScenario(t, client, spec, 1)
	for _, run := range result.Runs {
		printRunDetail(t, run)
	}
	printScenarioSummary(t, result)
	if !result.MeetsThreshold {
		t.Errorf("FAIL: passRate=%.0f%%", result.PassRate*100)
	}
}

func scenarioByName(name string) ScenarioSpec {
	for _, s := range allScenarios() {
		if s.Name == name {
			return s
		}
	}
	panic("scenario not found: " + name)
}

func TestLLM_CasualGreeting_NoTools(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S01_打招呼"))
}

func TestLLM_Thanks_NoTools(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S02_感谢"))
}

func TestLLM_CasualChat_NoTools(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S03_闲聊"))
}

func TestLLM_StatementWithoutRecordIntent_NoCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S04_陈述无意图"))
}

func TestLLM_WorkDiscussion_NoCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S05_工作讨论无意图"))
}

func TestLLM_TechStatement_NoCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S06_技术陈述无意图"))
}

func TestLLM_ExplicitRecord_ShouldCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S07_明确记录_帮我记"))
}

func TestLLM_RecordPhrase_ShouldCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S08_明确记录_记录下来"))
}

func TestLLM_SearchRequest_ShouldSearch(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S09_搜索_结论查询"))
}

func TestLLM_AskForRecord_ShouldSearch(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S10_搜索_有没有记录"))
}

func TestLLM_DateInjection_CorrectYear(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S11_时间感知"))
}

func TestLLM_SimilarityCheck(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S12_相似度检查"))
}

func TestLLM_AmbiguousInput_NoCreate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("S13_模糊输入"))
}

func TestLLM_MultiTurn_ConfirmDraft(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M01_确认保存"))
}

func TestLLM_MultiTurn_ConfirmDraft_Casual(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M02_确认口语化"))
}

func TestLLM_MultiTurn_RejectDraft(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M03_拒绝_不要了"))
}

func TestLLM_MultiTurn_RejectDraft_Discard(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M04_拒绝_丢弃"))
}

func TestLLM_MultiTurn_ChangeCategory(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M05_修改分类"))
}

func TestLLM_MultiTurn_ReviseDraft_AddDate(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M06_修订_补充日期"))
}

func TestLLM_MultiTurn_ReviseDraft_ChangeTitle(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M07_修订_改标题"))
}

func TestLLM_MultiTurn_ReviseDraft_AppendContent(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M08_修订_追加内容"))
}

func TestLLM_MultiTurn_ConfirmWithDraftID(t *testing.T) {
	client := skipIfNoLLM(t)
	runSingleScenario(t, client, scenarioByName("M09_指定ID确认"))
}
