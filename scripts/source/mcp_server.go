package main

import (
	"context"
	_ "embed"
	"errors"
	"path/filepath"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	managementResourceURI  = "ui://codex-auto-retry/management-panel"
	managementResourceMIME = "text/html;profile=mcp-app"
)

//go:embed ui/dist/panel.html
var managementPanelHTML string

type emptyToolInput struct{}

type setRetryPromptInput struct {
	Prompt string `json:"prompt" jsonschema:"fallback text used only when silent continuation is unsupported, from 1 to 500 characters"`
}

type setRetrySettingsInput struct {
	RetryPrompt           string `json:"retry_prompt" jsonschema:"fallback text used only when silent continuation is unsupported, from 1 to 500 characters"`
	MaxConsecutiveRetries int    `json:"max_consecutive_retries" jsonschema:"maximum retries without visible assistant progress, from 1 to 100"`
	MaxRecoveryAttempts   int    `json:"max_recovery_attempts" jsonschema:"maximum attempts in one fault recovery cycle, from 1 to 1000"`
	AuthMaxAttempts       *int   `json:"auth_max_attempts,omitempty" jsonschema:"additional ceiling for authentication errors, from 1 to 1000; omit to preserve the current value; both global limits still apply"`
	InitialDelaySeconds   int    `json:"initial_delay_seconds" jsonschema:"fixed delay or first increasing delay, from 1 to 3600 seconds"`
	MaxDelaySeconds       int    `json:"max_delay_seconds" jsonschema:"maximum increasing delay, from 1 to 86400 seconds"`
	DelayIncrementSeconds int    `json:"delay_increment_seconds" jsonschema:"seconds added after each linear retry, from 1 to 3600 seconds"`
	DelayStrategy         string `json:"delay_strategy" jsonschema:"fixed for a constant delay, exponential for doubling delays, or linear for a fixed increase each retry"`
	ShowNotifications     bool   `json:"show_notifications" jsonschema:"whether Windows retry notifications are enabled"`
	MemoryLimitMB         *int   `json:"memory_limit_mb,omitempty" jsonschema:"private memory limit for the watchdog process, from 128 to 65536 MB; omit to keep the current value"`
}

type setUILanguageInput struct {
	Language string `json:"language" jsonschema:"interface language: zh for Traditional Chinese or en for English"`
}

type setPausedInput struct {
	Paused bool `json:"paused" jsonschema:"true to pause new retry dispatches, false to resume them"`
}

type setSharedAppServerInput struct {
	Enabled bool `json:"enabled" jsonschema:"true only after the optional shared Codex app-server passes its ownership and health checks"`
}

type threadControlInput struct {
	ThreadID string `json:"thread_id" jsonschema:"Codex task identifier from the current retry queue"`
}

func runManagementMCP(dataDir string) error {
	service := newManagementService(dataDir)
	server := newManagementMCPServer(service)
	config, err := loadOrCreateConfig(filepath.Join(dataDir, "config.json"))
	if err != nil {
		config = defaultConfig()
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	memoryTriggered := make(chan memorySample, 1)
	guard := newProcessMemoryGuard(config.MemoryLimitMB, memoryCheckInterval, nil, nil, func(sample memorySample) {
		appendMemoryGuardLog(dataDir, "mcp", sample, config.MemoryLimitMB)
		select {
		case memoryTriggered <- sample:
		default:
		}
		cancel()
	})
	go guard.Run(ctx)
	err = server.Run(ctx, &mcp.StdioTransport{})
	select {
	case sample := <-memoryTriggered:
		showMemoryLimitAlert(sample, config.MemoryLimitMB)
	default:
	}
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func newManagementMCPServer(service *managementService) *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "codex-auto-retry",
		Title:   "Codex Auto Retry",
		Version: appVersion,
	}, nil)

	server.AddResource(&mcp.Resource{
		Meta:        managementResourceMeta(),
		URI:         managementResourceURI,
		Name:        "codex_auto_retry_panel",
		Title:       "Codex Auto Retry",
		Description: "Interactive local retry status and controls.",
		MIMEType:    managementResourceMIME,
	}, func(_ context.Context, _ *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{{
			URI:      managementResourceURI,
			MIMEType: managementResourceMIME,
			Text:     managementPanelHTML,
			Meta:     managementResourceMeta(),
		}}}, nil
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "get_auto_retry_status",
		Title:       "檢視自動重試 / View auto retry",
		Description: "檢視 Codex 自動重試服務、倒數計時、佇列和目前設定，並顯示管理面板。 / Show the Codex auto-retry service, countdowns, queue, and current settings in the management panel.",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: true, OpenWorldHint: boolPointer(false), Title: "檢視自動重試 / View auto retry"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyToolInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.snapshot(time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "set_retry_prompt",
		Title:       "修改後備重試文字 / Set fallback retry prompt",
		Description: "修改靜默續接不受支援時使用的後備文字；正常情況下不會新增可見訊息，目標模式仍使用原生恢復。 / Change the fallback text used only when silent continuation is unsupported; normally no visible message is added, and goal mode keeps native recovery.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "修改重試文字 / Set retry prompt"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input setRetryPromptInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.setRetryPrompt(input.Prompt, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "set_retry_settings",
		Title:       "修改自動重試設定 / Change retry settings",
		Description: "同時修改後備重試文字、連續無進展重試上限、單次故障恢復上限、固定、翻倍或線性等待策略，以及外掛達到上限時的通知設定。 / Change the fallback prompt, no-progress limit, per-outage recovery limit, fixed, exponential, or linear wait strategy, and the limit notification together.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "修改自動重試設定 / Change retry settings"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input setRetrySettingsInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		settings := RetrySettings{
			RetryPrompt:           input.RetryPrompt,
			MaxConsecutiveRetries: input.MaxConsecutiveRetries,
			MaxRecoveryAttempts:   input.MaxRecoveryAttempts,
			AuthMaxAttempts:       input.AuthMaxAttempts,
			InitialDelaySeconds:   input.InitialDelaySeconds,
			MaxDelaySeconds:       input.MaxDelaySeconds,
			DelayIncrementSeconds: input.DelayIncrementSeconds,
			DelayStrategy:         input.DelayStrategy,
			ShowNotifications:     input.ShowNotifications,
		}
		if input.MemoryLimitMB != nil {
			settings.MemoryLimitMB = *input.MemoryLimitMB
		}
		snapshot, err := service.setRetrySettings(settings, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "set_auto_retry_paused",
		Title:       "暫停或恢復自動重試 / Pause or resume auto retry",
		Description: "暫停或恢復新的自動重試；已開始的 Codex 任務不會被強制終止。 / Pause or resume new automatic retries; Codex tasks already running are never force-stopped.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "暫停或恢復自動重試 / Pause or resume auto retry"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input setPausedInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.setPaused(input.Paused, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "set_shared_app_server_enabled",
		Title:       "啟用或關閉共用後端 / Enable or disable shared backend",
		Description: "明確啟用或關閉可選的共用 Codex 後端。啟用前會檢查迴路埠、WebSocket 交握、執行檔和行程歸屬；失敗時不會修改 Codex 環境。 / Explicitly enable or disable the optional shared Codex backend. Enabling first checks the loopback port, WebSocket handshake, executable, and process ownership; on failure the Codex environment is left unchanged.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "共用後端模式 / Shared backend mode"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input setSharedAppServerInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.setSharedAppServerEnabled(input.Enabled, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "set_ui_language",
		Title:       "切換介面語言 / Set interface language",
		Description: "切換管理面板、設定視窗、啟動管理員與系統匣的介面語言（zh 繁體中文、en 英文）。 / Switch the language of the panel, settings window, startup manager, and tray (zh Traditional Chinese, en English).",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "切換介面語言 / Set interface language"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input setUILanguageInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.setUILanguage(input.Language, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "rescan_interrupted_tasks",
		Title:       "重新偵測中斷的任務 / Find interrupted tasks",
		Description: "掃描最近 24 小時的任務，把最後一輪因可重試錯誤而中斷、之後沒有再繼續的任務列入佇列等待使用者按重新開始；不會自動送出重試。 / Scan tasks from the last 24 hours and list those whose last turn ended with a retryable failure and was never continued; they wait for the user to restart them and are never retried automatically.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "重新偵測中斷的任務 / Find interrupted tasks"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, _ emptyToolInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.rescanInterrupted(time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "retry_now",
		Title:       "立即重試 / Retry now",
		Description: "讓佇列中指定 Codex 任務儘快重試；暫停期間會保持待處理，恢復後執行。 / Retry the queued Codex task as soon as possible; while paused it stays pending and runs after resuming.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false), Title: "立即重試 / Retry now"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input threadControlInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.retryNow(input.ThreadID, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "cancel_retry",
		Title:       "取消等待中的重試 / Cancel pending retry",
		Description: "取消佇列中指定任務尚未開始的重試；已經開始的 Codex 任務不會被中止。 / Cancel the not-yet-started retry of the queued task; Codex tasks already running are not interrupted.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), IdempotentHint: true, OpenWorldHint: boolPointer(false), Title: "取消等待中的重試 / Cancel pending retry"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input threadControlInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.cancelRetry(input.ThreadID, time.Now().UTC())
		return nil, snapshot, err
	})

	mcp.AddTool(server, &mcp.Tool{
		Meta:        managementToolMeta(),
		Name:        "restart_retry",
		Title:       "重新開始重試 / Restart retry",
		Description: "為已經達到任一重試上限的任務重新開始兩個計數並立即重試。 / Reset both counters for a task that hit a retry limit and retry it now.",
		Annotations: &mcp.ToolAnnotations{DestructiveHint: boolPointer(false), OpenWorldHint: boolPointer(false), Title: "重新開始重試 / Restart retry"},
	}, func(_ context.Context, _ *mcp.CallToolRequest, input threadControlInput) (*mcp.CallToolResult, ManagementSnapshot, error) {
		snapshot, err := service.restartRetry(input.ThreadID, time.Now().UTC())
		return nil, snapshot, err
	})

	return server
}

func managementToolMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"resourceUri": managementResourceURI,
			"visibility":  []string{"model", "app"},
		},
		"ui/resourceUri":          managementResourceURI,
		"openai/outputTemplate":   managementResourceURI,
		"openai/widgetAccessible": true,
	}
}

func managementResourceMeta() mcp.Meta {
	return mcp.Meta{
		"ui": map[string]any{
			"prefersBorder": true,
			"csp": map[string]any{
				"connectDomains":  []string{},
				"resourceDomains": []string{},
				"frameDomains":    []string{},
			},
		},
		"openai/widgetPrefersBorder": true,
		"openai/widgetCSP": map[string]any{
			"connect_domains":  []string{},
			"resource_domains": []string{},
		},
	}
}

func boolPointer(value bool) *bool {
	return &value
}
