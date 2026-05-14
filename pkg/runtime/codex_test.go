package runtime

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
)

func TestCodexCLIExecutorUsesHeadlessExecAndModel(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake uses /bin/sh")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	logPath := filepath.Join(dir, "args.txt")
	script := `#!/bin/sh
printf '%s\n' "$@" > "` + logPath + `"
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    printf 'optimized output\n' > "$1"
    exit 0
  fi
  shift
done
exit 2
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	exec := NewCodexCLIExecutor(CodexCLIConfig{Command: fake})
	out, err := exec.ExecuteAgent(context.Background(), ExecutionRequest{
		Defaults: model.Defaults{Model: "gpt-5.3-codex-spark"},
		Agent:    model.Agent{Name: "speed_optimizer"},
		Content:  "Return optimized code.",
		Inputs:   map[string]string{"code": "x = x + 1"},
	})
	if err != nil {
		t.Fatalf("ExecuteAgent: %v", err)
	}
	if out != "optimized output" {
		t.Fatalf("output = %q, want optimized output", out)
	}

	args, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read args log: %v", err)
	}
	argText := string(args)
	for _, want := range []string{
		"exec\n",
		"--sandbox\nread-only\n",
		"--ephemeral\n",
		"--model\ngpt-5.3-codex-spark\n",
		"-\n",
	} {
		if !strings.Contains(argText, want) {
			t.Fatalf("codex args missing %q:\n%s", want, argText)
		}
	}
}

func TestPromptRouterUsesConfiguredCodexExecutor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fake uses /bin/sh")
	}

	dir := t.TempDir()
	fake := filepath.Join(dir, "codex")
	script := `#!/bin/sh
while [ "$#" -gt 0 ]; do
  if [ "$1" = "--output-last-message" ]; then
    shift
    printf 'codex route\n' > "$1"
    exit 0
  fi
  shift
done
exit 2
`
	if err := os.WriteFile(fake, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake codex: %v", err)
	}

	router := NewPromptRouterExecutor(PromptRouterConfig{
		Codex: CodexCLIConfig{Command: fake},
		HTTP:  HTTPPromptConfig{AnthropicAPIKey: "", OpenAIAPIKey: ""},
	})
	out, err := router.ExecuteAgent(context.Background(), ExecutionRequest{
		Defaults: model.Defaults{
			PromptExecutor: PromptExecutorCodexCLI,
			Model:          "gpt-5.3-codex-spark",
		},
		Agent:   model.Agent{Name: "agent"},
		Content: "Return output.",
	})
	if err != nil {
		t.Fatalf("ExecuteAgent: %v", err)
	}
	if out != "codex route" {
		t.Fatalf("output = %q, want codex route", out)
	}
}
