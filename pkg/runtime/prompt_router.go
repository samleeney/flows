package runtime

import (
	"context"
	"fmt"
	"strings"
)

const (
	PromptExecutorAnthropicAPI = "anthropic_api"
	PromptExecutorOpenAIAPI    = "openai_api"
)

// PromptRouterConfig configures prompt execution backends.
type PromptRouterConfig struct {
	// DefaultExecutor overrides flow/agent prompt_executor when set. Accepted
	// values include codex_cli, codex_cli_write, anthropic_api, and openai_api.
	DefaultExecutor string

	HTTP  HTTPPromptConfig
	Codex CodexCLIConfig
}

// PromptRouterExecutor dispatches prompt nodes to the backend configured by
// agent.prompt_executor, defaults.prompt_executor, or CLI override.
type PromptRouterExecutor struct {
	defaultExecutor string
	anthropic       *HTTPPromptExecutor
	openai          *HTTPPromptExecutor
	codex           *CodexCLIExecutor
	codexWrite      *CodexCLIExecutor
}

func NewPromptRouterExecutor(cfg PromptRouterConfig) *PromptRouterExecutor {
	anthropicCfg := cfg.HTTP
	anthropicCfg.Provider = ProviderAnthropic
	openaiCfg := cfg.HTTP
	openaiCfg.Provider = ProviderOpenAI
	codexWriteCfg := cfg.Codex
	codexWriteCfg.Sandbox = DefaultCodexWriteSandbox
	codexWriteCfg.AllowEdits = true

	return &PromptRouterExecutor{
		defaultExecutor: normalizePromptExecutor(cfg.DefaultExecutor),
		anthropic:       NewHTTPPromptExecutor(anthropicCfg),
		openai:          NewHTTPPromptExecutor(openaiCfg),
		codex:           NewCodexCLIExecutor(cfg.Codex),
		codexWrite:      NewCodexCLIExecutor(codexWriteCfg),
	}
}

func (e *PromptRouterExecutor) Execute(ctx context.Context, content string, inputs map[string]string) (string, error) {
	return e.ExecuteAgent(ctx, ExecutionRequest{Content: content, Inputs: inputs})
}

func (e *PromptRouterExecutor) ExecuteAgent(ctx context.Context, req ExecutionRequest) (string, error) {
	executor := e.resolveExecutor(req)
	switch executor {
	case PromptExecutorCodexCLI:
		return e.codex.ExecuteAgent(ctx, req)
	case PromptExecutorCodexCLIWrite:
		return e.codexWrite.ExecuteAgent(ctx, req)
	case PromptExecutorAnthropicAPI:
		return e.anthropic.ExecuteAgent(ctx, req)
	case PromptExecutorOpenAIAPI:
		return e.openai.ExecuteAgent(ctx, req)
	case "":
		return e.executeInferredAPI(ctx, req)
	default:
		return "", fmt.Errorf("unsupported prompt_executor %q", executor)
	}
}

func (e *PromptRouterExecutor) resolveExecutor(req ExecutionRequest) string {
	if e.defaultExecutor != "" {
		return e.defaultExecutor
	}
	if req.Agent.PromptExecutor != "" {
		return normalizePromptExecutor(req.Agent.PromptExecutor)
	}
	return normalizePromptExecutor(req.Defaults.PromptExecutor)
}

func (e *PromptRouterExecutor) executeInferredAPI(ctx context.Context, req ExecutionRequest) (string, error) {
	model := strings.TrimSpace(req.Agent.Model)
	if model == "" {
		model = strings.TrimSpace(req.Defaults.Model)
	}
	switch inferProvider(model, e.anthropic.cfg) {
	case ProviderAnthropic:
		return e.anthropic.ExecuteAgent(ctx, req)
	case ProviderOpenAI:
		return e.openai.ExecuteAgent(ctx, req)
	default:
		return "", fmt.Errorf("prompt node %q has no prompt_executor; set defaults.prompt_executor, agent prompt_executor, or --prompt-executor", req.Agent.Name)
	}
}

func normalizePromptExecutor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "auto":
		return ""
	case PromptExecutorCodexCLI, PromptExecutorCodexHeadless, "codex", "codex_exec":
		return PromptExecutorCodexCLI
	case PromptExecutorCodexCLIWrite, "codex_write", "codex_edit", "codex_workspace_write":
		return PromptExecutorCodexCLIWrite
	case PromptExecutorAnthropicAPI, ProviderAnthropic, "anthropic_http":
		return PromptExecutorAnthropicAPI
	case PromptExecutorOpenAIAPI, ProviderOpenAI, "openai_http":
		return PromptExecutorOpenAIAPI
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
