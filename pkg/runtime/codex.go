package runtime

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	PromptExecutorCodexCLI      = "codex_cli"
	PromptExecutorCodexHeadless = "codex_headless"
	PromptExecutorCodexCLIWrite = "codex_cli_write"
	DefaultCodexCommand         = "codex"
	DefaultCodexModel           = "gpt-5.3-codex-spark"
	DefaultCodexSandbox         = "read-only"
	DefaultCodexWriteSandbox    = "workspace-write"
)

// CodexCLIConfig configures the headless Codex CLI prompt executor.
type CodexCLIConfig struct {
	Command    string
	Model      string
	WorkDir    string
	Timeout    time.Duration
	Sandbox    string
	AllowEdits bool
}

// CodexCLIExecutor executes prompt nodes through `codex exec`, using the
// local Codex login and configuration.
type CodexCLIExecutor struct {
	cfg CodexCLIConfig
}

func NewCodexCLIExecutor(cfg CodexCLIConfig) *CodexCLIExecutor {
	if cfg.Command == "" {
		cfg.Command = DefaultCodexCommand
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = DefaultPromptTimeout
	}
	if cfg.Sandbox == "" {
		cfg.Sandbox = DefaultCodexSandbox
	}
	return &CodexCLIExecutor{cfg: cfg}
}

func (e *CodexCLIExecutor) Execute(ctx context.Context, content string, inputs map[string]string) (string, error) {
	return e.ExecuteAgent(ctx, ExecutionRequest{Content: content, Inputs: inputs})
}

func (e *CodexCLIExecutor) ExecuteAgent(ctx context.Context, req ExecutionRequest) (string, error) {
	model := strings.TrimSpace(e.cfg.Model)
	if model == "" {
		model = strings.TrimSpace(req.Agent.Model)
	}
	if model == "" {
		model = strings.TrimSpace(req.Defaults.Model)
	}
	if model == "" {
		model = DefaultCodexModel
	}

	prompt := e.buildCodexPrompt(req)
	outFileHandle, err := os.CreateTemp("", "flow-codex-*.txt")
	if err != nil {
		return "", fmt.Errorf("creating codex output file: %w", err)
	}
	outFile := outFileHandle.Name()
	_ = outFileHandle.Close()
	defer os.Remove(outFile)

	runCtx, cancel := context.WithTimeout(ctx, e.cfg.Timeout)
	defer cancel()

	args := []string{
		"exec",
		"--sandbox", e.cfg.Sandbox,
		"--skip-git-repo-check",
		"--ephemeral",
		"--color", "never",
		"--output-last-message", outFile,
	}
	if model != "" {
		args = append(args, "--model", model)
	}
	args = append(args, "-")

	cmd := exec.CommandContext(runCtx, e.cfg.Command, args...)
	if e.cfg.WorkDir != "" {
		cmd.Dir = e.cfg.WorkDir
	}
	cmd.Stdin = strings.NewReader(prompt)
	combined, err := cmd.CombinedOutput()
	if runCtx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("codex exec timed out after %s", e.cfg.Timeout)
	}
	if err != nil {
		return "", fmt.Errorf("codex exec failed: %w\noutput: %s", err, strings.TrimSpace(string(combined)))
	}

	data, err := os.ReadFile(outFile)
	if err != nil {
		return "", fmt.Errorf("reading codex final response: %w\noutput: %s", err, strings.TrimSpace(string(combined)))
	}
	output := strings.TrimSpace(string(data))
	if output == "" {
		output = strings.TrimSpace(string(combined))
	}
	if output == "" {
		return "", fmt.Errorf("codex exec produced no output")
	}
	return output, nil
}

func (e *CodexCLIExecutor) buildCodexPrompt(req ExecutionRequest) string {
	systemPrompt, userPrompt := buildLLMPrompt(req)
	editPolicy := "Do not edit files. Do not apply patches. Return only the final node output requested by the node prompt."
	if e.cfg.AllowEdits {
		editPolicy = "You may edit files in the workspace only when the node prompt explicitly asks for file changes. Return a concise final node output describing the result requested by the node prompt."
	}
	return systemPrompt + "\n\n" + editPolicy + "\n\n" + userPrompt
}
