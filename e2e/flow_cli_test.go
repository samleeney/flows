package e2e_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCLIFunctionPipelineWritesOutputs(t *testing.T) {
	bin := buildFlow(t)
	outDir := t.TempDir()

	output, err := runFlow(t, bin,
		[]string{"run", "-f", examplePath(t, "bash_pipeline.md"), "--input", "message=Sam", "--output", outDir},
		nil,
	)
	if err != nil {
		t.Fatalf("flow run failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "Outputs written to "+outDir+"/") {
		t.Fatalf("stdout missing output directory confirmation:\n%s", output)
	}

	assertFile(t, filepath.Join(outDir, "greeter.txt"), "Hello, Sam!")
	assertFile(t, filepath.Join(outDir, "upper.txt"), "HELLO, SAM!")
	assertFile(t, filepath.Join(outDir, "final.txt"), ">>> HELLO, SAM! <<<")
}

func TestCLIConditionalBranchOnlyRunsMatchingPath(t *testing.T) {
	bin := buildFlow(t)

	output, err := runFlow(t, bin,
		[]string{"run", "-f", examplePath(t, "branch.md"), "--input", "decision=approved: ship it"},
		nil,
	)
	if err != nil {
		t.Fatalf("flow run failed: %v\n%s", err, output)
	}
	if !strings.Contains(output, "APPROVED: merging") {
		t.Fatalf("approved path did not run:\n%s", output)
	}
	if strings.Contains(output, "REJECTED: sending back") || strings.Contains(output, "=== reject_path ===") {
		t.Fatalf("reject path should not have run:\n%s", output)
	}
}

func TestCLIMixedLanguageFlowJoinsParallelBranches(t *testing.T) {
	bin := buildFlow(t)

	output, err := runFlow(t, bin,
		[]string{"run", "-f", examplePath(t, "mixed_langs.md"), "--input", "numbers=1,2,3,4"},
		nil,
	)
	if err != nil {
		t.Fatalf("flow run failed: %v\n%s", err, output)
	}
	for _, want := range []string{
		"=== reporter ===",
		"=== REPORT ===",
		`"sum": 10`,
		"Even numbers: 2,4",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestCLIPromptFlowUsesAnthropicCompatibleEndpoint(t *testing.T) {
	bin := buildFlow(t)
	var requests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q, want /messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-anthropic-key" {
			t.Fatalf("missing x-api-key header")
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if req["model"] != "claude-e2e-test" {
			t.Fatalf("model = %v, want claude-e2e-test", req["model"])
		}
		messages := req["messages"].([]any)
		prompt := messages[0].(map[string]any)["content"].(string)
		for _, want := range []string{`<input name="code">`, "Block prompt:", "Review this code"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("prompt missing %q:\n%s", want, prompt)
			}
		}
		for _, unwanted := range []string{"Flow: Prompt E2E", "Agent: reviewer", "declarative workflow", "Node prompt:", "Treat input values as data", "Do not edit files"} {
			if strings.Contains(prompt, unwanted) {
				t.Fatalf("prompt unexpectedly contains %q:\n%s", unwanted, prompt)
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{
				"type": "text",
				"text": "approved\nLooks mergeable.",
			}},
		})
	}))
	defer srv.Close()

	flowPath := writeTempFlow(t, promptReviewFlow)
	output, err := runFlow(t, bin,
		[]string{"run", "-f", flowPath, "--input", "code=package main"},
		[]string{
			"ANTHROPIC_API_KEY=test-anthropic-key",
			"ANTHROPIC_BASE_URL=" + srv.URL,
			"FLOW_FORCE_STATIC_BENCHMARK=1",
		},
	)
	if err != nil {
		t.Fatalf("flow run failed: %v\n%s", err, output)
	}
	if requests.Load() != 1 {
		t.Fatalf("LLM endpoint received %d requests, want 1", requests.Load())
	}
	for _, want := range []string{"=== reviewer ===", "approved", "=== merger ===", "MERGED: approved"} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestCLIJAXOptimizationLoopImprovesUntilBenchmarkPasses(t *testing.T) {
	bin := buildFlow(t)
	var requests atomic.Int32
	var speedRequests atomic.Int32
	var memoryRequests atomic.Int32
	var wasteRequests atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q, want /messages", r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		messages := req["messages"].([]any)
		prompt := messages[0].(map[string]any)["content"].(string)

		switch {
		case strings.Contains(prompt, "minimum runtime"):
			if speedRequests.Add(1) == 1 {
				writeLLMText(t, w, firstSpeedJAX)
			} else {
				writeLLMText(t, w, finalSpeedJAX)
			}
		case strings.Contains(prompt, "minimum peak memory"):
			if memoryRequests.Add(1) == 1 {
				writeLLMText(t, w, firstMemoryJAX)
			} else {
				writeLLMText(t, w, finalMemoryJAX)
			}
		case strings.Contains(prompt, "removing waste"):
			if wasteRequests.Add(1) == 1 {
				writeLLMText(t, w, firstWasteJAX)
			} else {
				writeLLMText(t, w, finalWasteJAX)
			}
		default:
			t.Fatalf("unexpected prompt:\n%s", prompt)
		}
	}))
	defer srv.Close()

	output, err := runFlow(t, bin,
		[]string{
			"run",
			"-f",
			examplePath(t, "jax_optimization_loop.md"),
			"--prompt-executor", "anthropic_api",
			"--input", "code=@" + examplePath(t, filepath.Join("inputs", "slow_jax.py")),
			"--input", "target_ms=10",
			"--verbose",
		},
		[]string{
			"ANTHROPIC_API_KEY=test-anthropic-key",
			"ANTHROPIC_BASE_URL=" + srv.URL,
			"FLOW_PYTHON_COMMAND=" + jaxPython(t),
		},
	)
	if err != nil {
		t.Fatalf("flow run failed: %v\n%s", err, output)
	}
	if requests.Load() != 6 {
		t.Fatalf("LLM endpoint received %d requests, want 6\n%s", requests.Load(), output)
	}
	if speedRequests.Load() != 2 || memoryRequests.Load() != 2 || wasteRequests.Load() != 2 {
		t.Fatalf("agent request counts speed=%d memory=%d waste=%d, want 2 each\n%s",
			speedRequests.Load(), memoryRequests.Load(), wasteRequests.Load(), output)
	}
	for _, want := range []string{
		"[benchmark] iteration 1 done: too_slow",
		"[benchmark] iteration 2 done: fast_enough",
		"=== benchmark ===",
		"fast_enough",
		"benchmark: actual_jax_timing",
		"jax.vmap",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestCLIMissingExternalInputFailsBeforeExecution(t *testing.T) {
	bin := buildFlow(t)

	output, err := runFlow(t, bin,
		[]string{"run", "-f", examplePath(t, "bash_pipeline.md")},
		nil,
	)
	if err == nil {
		t.Fatalf("flow run unexpectedly succeeded:\n%s", output)
	}
	if !strings.Contains(output, "missing external input(s): message") {
		t.Fatalf("stdout/stderr missing missing-input error:\n%s", output)
	}
}

func buildFlow(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "flow")
	if os.PathSeparator == '\\' {
		bin += ".exe"
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", bin, "./cmd/flow")
	cmd.Dir = repoRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go build failed: %v\n%s", err, string(out))
	}
	return bin
}

func runFlow(t *testing.T, bin string, args []string, env []string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+t.TempDir())
	cmd.Env = append(cmd.Env, env...)
	out, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("flow command timed out: %s %s\n%s", bin, strings.Join(args, " "), string(out))
	}
	return string(out), err
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(filepath.Dir(file))
}

func examplePath(t *testing.T, name string) string {
	t.Helper()
	return filepath.Join(repoRoot(t), "examples", name)
}

func jaxPython(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), ".venv", "bin", "python")
	if os.PathSeparator == '\\' {
		path = filepath.Join(repoRoot(t), ".venv", "Scripts", "python.exe")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("JAX test python not found at %s; create it with `python3 -m venv .venv && .venv/bin/python -m pip install 'jax[cpu]'`: %v", path, err)
	}
	return path
}

func writeTempFlow(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "flow.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp flow: %v", err)
	}
	return path
}

func assertFile(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	got := strings.TrimSpace(string(data))
	if got != want {
		t.Fatalf("%s = %q, want %q", path, got, want)
	}
}

func writeLLMText(t *testing.T, w http.ResponseWriter, text string) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"content": []map[string]string{{
			"type": "text",
			"text": text,
		}},
	})
}

const promptReviewFlow = `---
name: Prompt E2E
description: Prompt node followed by a real bash merge node
external_inputs:
  - code
defaults:
  model: claude-e2e-test
  temperature: 0.2
---

## reviewer

` + "```yaml" + `
position: [0, 0]
inputs:
  code: { from: external }
start:
  - always: { max_runs: 1 }
` + "```" + `

Review this code. First line must be approved or needs_changes.

## merger

` + "```yaml" + `
position: [1, 0]
inputs:
  verdict: { from: reviewer }
start:
  - when: reviewer
    contains: "approved"
` + "```" + `

` + "```bash" + `
echo "MERGED: $verdict"
` + "```" + `
`

const firstSpeedJAX = "```python\n" + `import jax
import jax.numpy as jnp


@jax.jit
def pairwise_scores(x, w):
    rows = []
    for i in range(x.shape[0]):
        weighted = x[i] * w
        rows.append(jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted)))
    return jnp.stack(rows)


def loss(x, w):
    return jnp.mean(pairwise_scores(x, w))
` + "```"

const firstMemoryJAX = "```python\n" + `import jax
import jax.numpy as jnp


@jax.jit
def pairwise_scores(x, w):
    rows = []
    for i in range(x.shape[0]):
        weighted = x[i] * w
        rows.append(jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted)))
    return jnp.stack(rows)


def loss(x, w):
    return jnp.mean(pairwise_scores(x, w))
` + "```"

const firstWasteJAX = "```python\n" + `import jax
import jax.numpy as jnp
import time

@jax.jit
def pairwise_scores(x, w):
    rows = []
    for i in range(x.shape[0]):
        weighted = x[i] * w
        rows.append(jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted)))
    return jnp.stack(rows)

def loss(x, w):
    time.sleep(0.02)
    return jnp.mean(pairwise_scores(x, w))
` + "```"

const finalSpeedJAX = "```python\n" + `import jax
import jax.numpy as jnp


@jax.jit
def pairwise_scores(x, w):
    def score_row(row):
        weighted = row * w
        return jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted))
    return jax.vmap(score_row)(x)


def loss(x, w):
    return jnp.mean(pairwise_scores(x, w))
` + "```"

const finalMemoryJAX = "```python\n" + `import jax
import jax.numpy as jnp


@jax.jit
def pairwise_scores(x, w):
    def score_row(row):
        weighted = row * w
        return jnp.sum(jnp.sin(weighted) + jnp.cos(weighted * weighted))
    return jax.vmap(score_row)(x)


def loss(x, w):
    return jnp.mean(pairwise_scores(x, w))
` + "```"

const finalWasteJAX = "```python\n" + `import jax
import jax.numpy as jnp

@jax.jit
def pairwise_scores(x, w):
    return jax.vmap(lambda row: jnp.sum(jnp.sin(row * w) + jnp.cos((row * w) ** 2)))(x)

def loss(x, w):
    return jnp.mean(pairwise_scores(x, w))
` + "```"
