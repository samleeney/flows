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

	"github.com/samleeney/flows/pkg/clioutput"
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

func TestCLIMachineFriendlyContract(t *testing.T) {
	bin := buildFlow(t)
	bashFlow := examplePath(t, "bash_pipeline.md")
	cacheDir := t.TempDir()

	t.Run("validate JSON and stdin", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"validate", bashFlow, "--json"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("validate: %v\nstdout: %s\nstderr: %s", err, stdout, stderr)
		}
		response := decodeResponse(t, stdout)
		if !response.OK || response.SchemaVersion != 1 || response.Command != "validate" || response.Flow == nil || response.Flow.BlockCount != 3 {
			t.Fatalf("unexpected response: %#v", response)
		}
		if stderr != "" {
			t.Fatalf("machine stderr = %q", stderr)
		}

		source, readErr := os.ReadFile(bashFlow)
		if readErr != nil {
			t.Fatal(readErr)
		}
		stdout, stderr, err = runFlowSeparate(t, bin, []string{"validate", "-", "--json"}, string(source), nil, cacheDir)
		if err != nil {
			t.Fatalf("stdin validate: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Flow == nil || response.Flow.Source != "<stdin>" {
			t.Fatalf("stdin source = %#v", response.Flow)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", "-", "--json", "--input", "message=stdin-flow"}, string(source), nil, cacheDir)
		if err != nil {
			t.Fatalf("stdin run: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Flow == nil || response.Flow.Source != "<stdin>" || response.Run == nil || response.Run.Outputs[0].Value != "Hello, stdin-flow!" {
			t.Fatalf("stdin run response = %#v", response)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"viz", "-"}, string(source), nil, cacheDir)
		if err != nil || !strings.HasPrefix(stdout, "graph LR") || stderr != "" {
			t.Fatalf("stdin viz: err=%v stdout=%q stderr=%q", err, stdout, stderr)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"validate", "-", "--json"}, "not a flow", nil, cacheDir)
		assertExitCode(t, err, 2)
		response = decodeResponse(t, stdout)
		if response.OK || response.Error == nil || len(response.Error.Diagnostics) == 0 || response.Error.Diagnostics[0].Phase != "parse" || response.Error.Diagnostics[0].Source != "<stdin>" {
			t.Fatalf("invalid stdin response: %#v", response)
		}
		if stderr != "" {
			t.Fatalf("invalid machine stderr = %q", stderr)
		}
	})

	t.Run("inspect exposes functions loops fallbacks and goals", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"inspect", examplePath(t, "counter_loop.md"), "--json"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("inspect loop: %v\n%s\n%s", err, stdout, stderr)
		}
		response := decodeResponse(t, stdout)
		if response.Flow == nil || len(response.Flow.Blocks) != 3 || response.Flow.Blocks[0].Kind != "function" {
			t.Fatalf("loop blocks: %#v", response.Flow)
		}
		foundFallback, foundStart := false, false
		for _, edge := range response.Flow.Edges {
			foundFallback = foundFallback || edge.Kind == "fallback" && edge.From == "external" && edge.To == "counter"
			foundStart = foundStart || edge.Kind == "start" && edge.From == "incrementer" && edge.To == "counter"
		}
		if !foundFallback || !foundStart {
			t.Fatalf("loop edges missing fallback/start: %#v", response.Flow.Edges)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"inspect", examplePath(t, "simple_goal.md"), "--json"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("inspect goal: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Flow == nil || len(response.Flow.Blocks) != 1 || response.Flow.Blocks[0].Goal == nil || response.Flow.Blocks[0].Goal.Objective == "" {
			t.Fatalf("goal missing: %#v", response.Flow)
		}

		functionFlow := filepath.Join(repoRoot(t), "pkg", "parser", "testdata", "function_node.md")
		stdout, stderr, err = runFlowSeparate(t, bin, []string{"inspect", functionFlow, "--json"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("inspect function: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Flow.Blocks[0].Kind != "function" || response.Flow.Blocks[0].Language != "python" || response.Flow.Blocks[0].Content == "" {
			t.Fatalf("function block = %#v", response.Flow.Blocks[0])
		}
	})

	t.Run("dry run validates required inputs", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--dry-run", "--json"}, "", nil, cacheDir)
		assertExitCode(t, err, 2)
		response := decodeResponse(t, stdout)
		if response.OK || response.Error == nil || response.Error.Code != "missing_inputs" || stderr != "" {
			t.Fatalf("missing input result=%#v stderr=%q", response, stderr)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", bashFlow, "--dry-run", "--json", "--input", "message=Sam"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("dry run: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if !response.OK || response.Mode != "dry_run" || len(response.RequiredInputs) != 1 || len(response.SuppliedInputs) != 1 {
			t.Fatalf("dry run result = %#v", response)
		}
	})

	t.Run("JSON inputs support multiline precedence stdin and errors", func(t *testing.T) {
		inputsPath := filepath.Join(t.TempDir(), "inputs.json")
		if err := os.WriteFile(inputsPath, []byte(`{"message":"from\njson"}`), 0o644); err != nil {
			t.Fatal(err)
		}
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--inputs-json", inputsPath}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("JSON inputs: %v\n%s\n%s", err, stdout, stderr)
		}
		response := decodeResponse(t, stdout)
		if response.Run == nil || len(response.Run.Outputs) != 3 || !strings.Contains(response.Run.Outputs[0].Value, "from\njson") {
			t.Fatalf("multiline input result = %#v", response.Run)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--inputs-json", inputsPath, "--input", "message=override"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("input override: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Run.Outputs[0].Value != "Hello, override!" {
			t.Fatalf("override output = %q", response.Run.Outputs[0].Value)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--inputs-json", "-"}, `{"message":"stdin"}`, nil, cacheDir)
		if err != nil {
			t.Fatalf("stdin JSON input: %v\n%s\n%s", err, stdout, stderr)
		}
		response = decodeResponse(t, stdout)
		if response.Run.Outputs[0].Value != "Hello, stdin!" {
			t.Fatalf("stdin output = %q", response.Run.Outputs[0].Value)
		}

		for name, stdin := range map[string]string{"malformed": `{`, "non_string": `{"message":3}`, "non_object": `null`} {
			t.Run(name, func(t *testing.T) {
				stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--inputs-json", "-"}, stdin, nil, cacheDir)
				assertExitCode(t, err, 2)
				response := decodeResponse(t, stdout)
				if response.Error == nil || response.Error.Code != "invalid_inputs" || stderr != "" {
					t.Fatalf("result=%#v stderr=%q", response, stderr)
				}
			})
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", "-", "--json", "--inputs-json", "-"}, "ignored", nil, cacheDir)
		assertExitCode(t, err, 2)
		response = decodeResponse(t, stdout)
		if response.Error == nil || response.Error.Code != "stdin_conflict" || stderr != "" {
			t.Fatalf("stdin conflict=%#v stderr=%q", response, stderr)
		}
	})

	t.Run("single JSON run is deterministic and reports files", func(t *testing.T) {
		outputDir := t.TempDir()
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--input", "message=Sam", "--output", outputDir}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("JSON run: %v\n%s\n%s", err, stdout, stderr)
		}
		response := decodeResponse(t, stdout)
		if !response.OK || response.Run == nil || response.Run.RunID == "" || response.Run.StartedAt.IsZero() || response.Run.FinishedAt.Before(response.Run.StartedAt) {
			t.Fatalf("run metadata = %#v", response.Run)
		}
		wantOrder := []string{"greeter", "upper", "final"}
		if len(response.Run.Outputs) != len(wantOrder) {
			t.Fatalf("outputs = %#v", response.Run.Outputs)
		}
		for i, want := range wantOrder {
			if response.Run.Outputs[i].Block != want || response.Run.Outputs[i].Path != filepath.Join(outputDir, want+".txt") {
				t.Fatalf("output %d = %#v", i, response.Run.Outputs[i])
			}
		}
		if response.Run.OutputDir != outputDir || stderr != "" {
			t.Fatalf("output directory=%q stderr=%q", response.Run.OutputDir, stderr)
		}
	})

	t.Run("JSONL streams ordered events and terminal outputs", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--jsonl", "--input", "message=Sam"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("JSONL run: %v\n%s\n%s", err, stdout, stderr)
		}
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		if len(lines) < 4 {
			t.Fatalf("too few events: %s", stdout)
		}
		var previous uint64
		var runID string
		for i, line := range lines {
			var event clioutput.Event
			if err := json.Unmarshal([]byte(line), &event); err != nil {
				t.Fatalf("line %d is not JSON: %v\n%s", i, err, line)
			}
			if event.SchemaVersion != 1 || event.Command != "run" || event.Sequence <= previous || event.RunID == "" {
				t.Fatalf("bad event %d: %#v", i, event)
			}
			if runID != "" && event.RunID != runID {
				t.Fatalf("run id changed from %s to %s", runID, event.RunID)
			}
			runID, previous = event.RunID, event.Sequence
		}
		var first, last clioutput.Event
		_ = json.Unmarshal([]byte(lines[0]), &first)
		_ = json.Unmarshal([]byte(lines[len(lines)-1]), &last)
		if first.Event != "run_started" || last.Event != "run_finished" || !last.OK || last.Outputs == nil || len(*last.Outputs) != 3 || stderr != "" {
			t.Fatalf("first=%#v last=%#v stderr=%q", first, last, stderr)
		}
	})

	t.Run("branching JSON includes only the selected path", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", examplePath(t, "branch.md"), "--json", "--input", "decision=approved: ship it"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("branch run: %v\n%s\n%s", err, stdout, stderr)
		}
		response := decodeResponse(t, stdout)
		if response.Run == nil {
			t.Fatalf("missing branch run: %#v", response)
		}
		for _, output := range response.Run.Outputs {
			if output.Block == "reject_path" {
				t.Fatalf("unselected branch ran: %#v", response.Run.Outputs)
			}
		}
	})

	t.Run("execution failures use exit three", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", examplePath(t, "failure.md"), "--json", "--input", "input=value"}, "", nil, cacheDir)
		assertExitCode(t, err, 3)
		response := decodeResponse(t, stdout)
		if response.OK || response.Error == nil || response.Error.Code != "execution_failed" || response.Run == nil || len(response.Run.Outputs) != 1 || response.Run.Outputs[0].Block != "good_one" || stderr != "" {
			t.Fatalf("failure result=%#v stderr=%q", response, stderr)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", examplePath(t, "failure.md"), "--jsonl", "--input", "input=value"}, "", nil, cacheDir)
		assertExitCode(t, err, 3)
		lines := strings.Split(strings.TrimSpace(stdout), "\n")
		var penultimate, terminal clioutput.Event
		if len(lines) < 2 {
			t.Fatalf("failed JSONL output = %q", stdout)
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-2]), &penultimate); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal([]byte(lines[len(lines)-1]), &terminal); err != nil {
			t.Fatal(err)
		}
		if penultimate.Event != "execution_error" || terminal.Event != "run_finished" || terminal.OK || terminal.Error == nil || terminal.Sequence <= penultimate.Sequence || stderr != "" {
			t.Fatalf("penultimate=%#v terminal=%#v stderr=%q", penultimate, terminal, stderr)
		}

		stdout, stderr, err = runFlowSeparate(t, bin, []string{"run", bashFlow, "--json", "--jsonl", "--input", "message=Sam"}, "", nil, cacheDir)
		assertExitCode(t, err, 2)
		var conflict clioutput.Event
		if decodeErr := json.Unmarshal([]byte(stdout), &conflict); decodeErr != nil || conflict.Event != "execution_error" || conflict.Error == nil || conflict.Error.Code != "invalid_arguments" || stderr != "" {
			t.Fatalf("conflict=%#v decode=%v stderr=%q stdout=%q", conflict, decodeErr, stderr, stdout)
		}
	})

	t.Run("no editor text mode", func(t *testing.T) {
		stdout, stderr, err := runFlowSeparate(t, bin, []string{"run", bashFlow, "--foreground", "--no-editor", "--input", "message=Sam"}, "", nil, cacheDir)
		if err != nil {
			t.Fatalf("no-editor run: %v\n%s\n%s", err, stdout, stderr)
		}
		if strings.Contains(stdout, "View flow:") || !strings.Contains(stdout, "=== final ===") {
			t.Fatalf("unexpected text output: %s", stdout)
		}
	})

	assertNoFiles(t, cacheDir)
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

func runFlowSeparate(t *testing.T, bin string, args []string, stdin string, env []string, cacheDir string) (string, string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = repoRoot(t)
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+cacheDir)
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("flow command timed out: %s %s\nstdout: %s\nstderr: %s", bin, strings.Join(args, " "), stdout.String(), stderr.String())
	}
	return stdout.String(), stderr.String(), err
}

func decodeResponse(t *testing.T, stdout string) clioutput.Response {
	t.Helper()
	var response clioutput.Response
	if err := json.Unmarshal([]byte(stdout), &response); err != nil {
		t.Fatalf("stdout is not exactly one JSON document: %v\n%s", err, stdout)
	}
	return response
}

func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("command succeeded, want exit %d", want)
	}
	exitErr, ok := err.(*exec.ExitError)
	if !ok || exitErr.ExitCode() != want {
		t.Fatalf("exit error = %v, want exit %d", err, want)
	}
}

func assertNoFiles(t *testing.T, root string) {
	t.Helper()
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path != root && !info.IsDir() {
			t.Errorf("unexpected editor or run artifact: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
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
