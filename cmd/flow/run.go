package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/model"
	"github.com/samleeney/flows/pkg/runtime"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		inputs      []string
		inputsJSON  string
		foreground  bool
		verbose     bool
		dryRun      bool
		outDir      string
		jsonOutput  bool
		jsonlOutput bool
		noEditor    bool

		promptExecutor string
		llmProvider    string
		llmModel       string
		maxTokens      int
		llmTimeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "run <file>",
		Short: "Start a flow run",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			machine := jsonOutput || jsonlOutput
			if jsonOutput && jsonlOutput {
				return failStructuredRun(cmd, true, "invalid_arguments", "--json and --jsonl are mutually exclusive", nil, exitInvalid)
			}
			if jsonlOutput && dryRun {
				return failStructuredRun(cmd, true, "invalid_arguments", "--jsonl cannot be used with --dry-run; use --json", nil, exitInvalid)
			}
			if args[0] == "-" && inputsJSON == "-" {
				message := "stdin cannot supply both the flow source and --inputs-json"
				if machine {
					return failStructuredRun(cmd, jsonlOutput, "stdin_conflict", message, nil, exitInvalid)
				}
				return errors.New(message)
			}

			flow, source, diagnostics, err := parseAndValidate(cmd, args[0])
			if err != nil {
				if machine {
					return failStructuredRun(cmd, jsonlOutput, "invalid_flow", fmt.Sprintf("%s failed for %s", diagnostics[0].Phase, source), diagnostics, exitInvalid)
				}
				if diagnostics[0].Phase == "parse" {
					if source == "<stdin>" {
						return fmt.Errorf("parse error (%s): %w", source, err)
					}
					return fmt.Errorf("parse error: %w", err)
				}
				return fmt.Errorf("validation failed:\n%w", err)
			}

			externalInputs, externalInputOrigins, err := loadExternalInputs(cmd, inputsJSON, inputs)
			if err != nil {
				if machine {
					return failStructuredRun(cmd, jsonlOutput, "invalid_inputs", err.Error(), nil, exitInvalid)
				}
				return err
			}

			missing := missingExternalInputs(flow, externalInputs)
			if dryRun && jsonOutput {
				if len(missing) > 0 {
					return failStructuredRun(cmd, false, "missing_inputs", "missing external input(s): "+strings.Join(missing, ", "), nil, exitInvalid)
				}
				summary := clioutput.FlowSummary(source, flow)
				return writeJSON(cmd, clioutput.Response{
					SchemaVersion:  clioutput.SchemaVersion,
					Command:        "run",
					OK:             true,
					Flow:           &summary,
					Mode:           "dry_run",
					RequiredInputs: append([]string(nil), flow.ExternalInputs...),
					SuppliedInputs: sortedInputNames(externalInputs),
				})
			}
			if dryRun {
				fmt.Fprintf(cmd.OutOrStdout(), "Flow: %s\n", flow.Name)
				fmt.Fprintf(cmd.OutOrStdout(), "Agents: %d\n", len(flow.Agents))
				fmt.Fprintf(cmd.OutOrStdout(), "External inputs: %v\n", flow.ExternalInputs)
				fmt.Fprintln(cmd.OutOrStdout(), "Dry run — not executing.")
				return nil
			}
			if len(missing) > 0 {
				message := "missing external input(s): " + strings.Join(missing, ", ")
				if machine {
					return failStructuredRun(cmd, jsonlOutput, "missing_inputs", message, nil, exitInvalid)
				}
				return errors.New(message)
			}

			effectiveForeground := foreground || machine || args[0] == "-"
			effectiveNoEditor := noEditor || machine || args[0] == "-"
			canonical := source
			if args[0] != "-" {
				canonical, err = live.CanonicalFlowPath(args[0])
				if err != nil {
					return handleRunInternalError(cmd, jsonOutput, jsonlOutput, fmt.Errorf("canonicalize: %w", err))
				}
			}
			flowKey := live.FlowKey(canonical)

			editorSession := runEditorSession{}
			if !effectiveNoEditor {
				editorSession, err = ensureRunEditor(args[0], canonical, flowKey, !effectiveForeground)
				if err != nil {
					return err
				}
				defer editorSession.Close()
				fmt.Fprintf(cmd.OutOrStdout(), "View flow: %s\n", editorSession.BaseURL)
			}

			if !effectiveForeground {
				pid, logPath, err := startBackgroundForegroundRun(flowKey)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Run started in background (pid %d).\n", pid)
				fmt.Fprintf(cmd.OutOrStdout(), "Log: %s\n", logPath)
				return nil
			}

			defaultPromptExecutor := promptExecutor
			if defaultPromptExecutor == "" && llmProvider != "" {
				defaultPromptExecutor = llmProvider
			}
			prompt := runtime.NewPromptRouterExecutor(runtime.PromptRouterConfig{
				DefaultExecutor: defaultPromptExecutor,
				HTTP: runtime.HTTPPromptConfig{
					Provider:         llmProvider,
					Model:            llmModel,
					AnthropicAPIKey:  os.Getenv("ANTHROPIC_API_KEY"),
					OpenAIAPIKey:     os.Getenv("OPENAI_API_KEY"),
					AnthropicBaseURL: os.Getenv("ANTHROPIC_BASE_URL"),
					OpenAIBaseURL:    os.Getenv("OPENAI_BASE_URL"),
					AnthropicVersion: os.Getenv("ANTHROPIC_VERSION"),
					MaxTokens:        maxTokens,
					Timeout:          llmTimeout,
				},
				Codex: runtime.CodexCLIConfig{Command: os.Getenv("FLOW_CODEX_COMMAND"), Model: llmModel, Timeout: llmTimeout},
			})
			registry := runtime.NewExecutorRegistry(prompt, &runtime.BashExecutor{}, &runtime.PythonExecutor{})

			var observer live.Observer = live.NopObserver{}
			var streamObserver *jsonlObserver
			switch {
			case jsonlOutput:
				streamObserver = newJSONLObserver(cmd.OutOrStdout())
				observer = streamObserver
			case !effectiveNoEditor:
				observer = buildLiveObserver(editorSession.Descriptors)
			}
			defer observer.Close()

			opts := runtime.RunOptions{
				ExternalInputs:       externalInputs,
				ExternalInputOrigins: externalInputOrigins,
				Verbose:              verbose,
				FlowKey:              flowKey,
				Observer:             observer,
			}
			if !machine && (effectiveForeground || verbose) {
				opts.OnAgentStart = func(name string, iter int) {
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] iteration %d starting...\n", name, iter)
				}
				opts.OnAgentDone = func(name string, iter int, output string, runErr error) {
					if runErr != nil {
						fmt.Fprintf(cmd.OutOrStdout(), "[%s] iteration %d FAILED: %v\n", name, iter, runErr)
						return
					}
					preview := output
					if len(preview) > 100 {
						preview = preview[:100] + "..."
					}
					fmt.Fprintf(cmd.OutOrStdout(), "[%s] iteration %d done: %s\n", name, iter, preview)
				}
			}

			result, executionErr := runtime.Run(context.Background(), flow, registry, opts)
			if !machine && executionErr != nil {
				return fmt.Errorf("execution failed: %w", executionErr)
			}
			outputs, outputErr := collectAndWriteOutputs(flow, result, outDir)
			if outputErr != nil {
				if jsonlOutput {
					if finishErr := streamObserver.finish(result, outputs, outputErr, "internal_error"); finishErr != nil {
						return &commandError{err: finishErr, exitCode: exitInternal}
					}
					return &commandError{err: outputErr, exitCode: exitInternal, emitted: true}
				}
				if jsonOutput {
					return writeRunJSON(cmd, source, flow, result, outputs, outDir, outputErr, exitInternal)
				}
				return outputErr
			}

			if jsonlOutput {
				if err := streamObserver.finish(result, outputs, executionErr, "execution_failed"); err != nil {
					return &commandError{err: err, exitCode: exitInternal}
				}
				if executionErr != nil {
					return &commandError{err: executionErr, exitCode: exitExecution, emitted: true}
				}
				return nil
			}
			if jsonOutput {
				exitCode := 0
				if executionErr != nil {
					exitCode = exitExecution
				}
				return writeRunJSON(cmd, source, flow, result, outputs, outDir, executionErr, exitCode)
			}
			if outDir != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Outputs written to %s/\n", outDir)
			} else {
				for _, output := range outputs {
					fmt.Fprintf(cmd.OutOrStdout(), "=== %s ===\n%s\n\n", output.Block, output.Value)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringArrayVar(&inputs, "input", nil, "External input as name=value (repeatable; overrides --inputs-json)")
	cmd.Flags().StringVar(&inputsJSON, "inputs-json", "", "Load external inputs from a JSON object at path or - for stdin")
	cmd.Flags().BoolVarP(&foreground, "foreground", "f", false, "Run in the foreground and print live progress")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print agent execution details")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and show plan without running")
	cmd.Flags().StringVar(&outDir, "output", "", "Directory to write agent outputs")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Run headlessly and emit one versioned JSON result")
	cmd.Flags().BoolVar(&jsonlOutput, "jsonl", false, "Run headlessly and stream versioned JSON Lines events")
	cmd.Flags().BoolVar(&noEditor, "no-editor", false, "Do not discover or start the browser editor")
	cmd.Flags().StringVar(&promptExecutor, "prompt-executor", os.Getenv("FLOW_PROMPT_EXECUTOR"), "Override prompt executor for all prompt nodes: codex_cli, codex_cli_write, anthropic_api, or openai_api")
	cmd.Flags().StringVar(&llmProvider, "llm-provider", os.Getenv("FLOW_LLM_PROVIDER"), "LLM provider for prompt nodes: anthropic or openai (default: infer from model)")
	cmd.Flags().StringVar(&llmModel, "model", os.Getenv("FLOW_MODEL"), "Override model for all prompt nodes (default: flow/agent model)")
	cmd.Flags().IntVar(&maxTokens, "max-tokens", envInt("FLOW_MAX_TOKENS", runtime.DefaultMaxTokens), "Maximum output tokens for prompt nodes")
	cmd.Flags().DurationVar(&llmTimeout, "llm-timeout", envDuration("FLOW_LLM_TIMEOUT", 0), "Override the backend timeout for each prompt-node LLM request")
	return cmd
}

func failStructuredRun(cmd *cobra.Command, jsonl bool, code, message string, diagnostics []clioutput.Diagnostic, exitCode int) error {
	if !jsonl {
		return failJSON(cmd, "run", code, message, diagnostics, exitCode)
	}
	return failJSONL(cmd, code, message, diagnostics, exitCode)
}

func handleRunInternalError(cmd *cobra.Command, jsonOutput, jsonlOutput bool, err error) error {
	if jsonOutput || jsonlOutput {
		return failStructuredRun(cmd, jsonlOutput, "internal_error", err.Error(), nil, exitInternal)
	}
	return err
}

func loadExternalInputs(cmd *cobra.Command, jsonSource string, overrides []string) (map[string]string, map[string]live.ExternalInputOrigin, error) {
	values := make(map[string]string)
	origins := make(map[string]live.ExternalInputOrigin)
	if jsonSource != "" {
		var reader io.Reader
		if jsonSource == "-" {
			reader = cmd.InOrStdin()
		} else {
			file, err := os.Open(jsonSource)
			if err != nil {
				return nil, nil, fmt.Errorf("reading --inputs-json %s: %w", jsonSource, err)
			}
			defer file.Close()
			reader = file
		}
		decoder := json.NewDecoder(reader)
		if err := decoder.Decode(&values); err != nil {
			return nil, nil, fmt.Errorf("decoding --inputs-json %s: expected an object with string values: %w", inputSourceLabel(jsonSource), err)
		}
		if values == nil {
			return nil, nil, fmt.Errorf("decoding --inputs-json %s: expected an object with string values", inputSourceLabel(jsonSource))
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			if err == nil {
				return nil, nil, fmt.Errorf("decoding --inputs-json %s: multiple JSON values are not allowed", inputSourceLabel(jsonSource))
			}
			return nil, nil, fmt.Errorf("decoding --inputs-json %s: %w", inputSourceLabel(jsonSource), err)
		}
		for name, value := range values {
			origins[name] = externalInputOriginFromJSON(name, jsonSource, value)
		}
	}

	for _, kv := range overrides {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, nil, fmt.Errorf("invalid input format %q, expected name=value", kv)
		}
		name, value := parts[0], parts[1]
		if strings.HasPrefix(value, "@") {
			path := value[1:]
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, nil, fmt.Errorf("reading input file for %q: %w", name, err)
			}
			value = string(data)
			origins[name] = externalInputOriginFromFile(name, path, value)
		} else {
			origins[name] = externalInputOriginFromInline(name, value)
		}
		values[name] = value
	}
	return values, origins, nil
}

func inputSourceLabel(source string) string {
	if source == "-" {
		return "<stdin>"
	}
	return source
}

func externalInputOriginFromInline(name, value string) live.ExternalInputOrigin {
	preview, total, truncated := live.TruncatePreviewUTF8(value, runtime.PreviewMaxBytes)
	return live.ExternalInputOrigin{Name: name, Source: "inline", Bytes: total, Preview: preview, PreviewTruncated: truncated}
}

func externalInputOriginFromFile(name, path, value string) live.ExternalInputOrigin {
	preview, total, truncated := live.TruncatePreviewUTF8(value, runtime.PreviewMaxBytes)
	abs, err := filepath.Abs(path)
	if err == nil {
		path = filepath.Clean(abs)
	}
	return live.ExternalInputOrigin{Name: name, Source: "file", Path: path, FileName: filepath.Base(path), Bytes: total, Preview: preview, PreviewTruncated: truncated}
}

func externalInputOriginFromJSON(name, source, value string) live.ExternalInputOrigin {
	preview, total, truncated := live.TruncatePreviewUTF8(value, runtime.PreviewMaxBytes)
	origin := live.ExternalInputOrigin{Name: name, Source: "json", Bytes: total, Preview: preview, PreviewTruncated: truncated}
	if source != "-" {
		if abs, err := filepath.Abs(source); err == nil {
			origin.Path = filepath.Clean(abs)
		} else {
			origin.Path = source
		}
		origin.FileName = filepath.Base(source)
	}
	return origin
}

func missingExternalInputs(flow *model.Flow, inputs map[string]string) []string {
	var missing []string
	for _, name := range flow.ExternalInputs {
		if _, ok := inputs[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func sortedInputNames(inputs map[string]string) []string {
	names := make([]string, 0, len(inputs))
	for name := range inputs {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func collectAndWriteOutputs(flow *model.Flow, result *runtime.RunResult, outDir string) ([]clioutput.Output, error) {
	outputs := make([]clioutput.Output, 0)
	if result == nil {
		return outputs, nil
	}
	if outDir != "" {
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return outputs, fmt.Errorf("creating output directory: %w", err)
		}
	}
	for _, agent := range flow.Agents {
		value, ok := result.Outputs[agent.Name]
		if !ok {
			continue
		}
		output := clioutput.Output{Block: agent.Name, Value: value}
		if outDir != "" {
			path := filepath.Join(outDir, agent.Name+".txt")
			if err := os.WriteFile(path, []byte(value), 0o644); err != nil {
				return outputs, fmt.Errorf("writing output for %q: %w", agent.Name, err)
			}
			output.Path = path
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

func writeRunJSON(cmd *cobra.Command, source string, flow *model.Flow, result *runtime.RunResult, outputs []clioutput.Output, outDir string, runErr error, exitCode int) error {
	summary := clioutput.FlowSummary(source, flow)
	response := clioutput.Response{SchemaVersion: clioutput.SchemaVersion, Command: "run", OK: runErr == nil, Flow: &summary, Mode: "foreground"}
	if result != nil {
		response.Run = &clioutput.Run{
			RunID:      result.RunID,
			StartedAt:  result.StartedAt,
			FinishedAt: result.FinishedAt,
			DurationMS: result.FinishedAt.Sub(result.StartedAt).Milliseconds(),
			Outputs:    outputs,
			OutputDir:  outDir,
		}
	}
	if runErr != nil {
		code := "execution_failed"
		if exitCode == exitInternal {
			code = "internal_error"
		}
		response.Error = &clioutput.Error{Code: code, Message: runErr.Error()}
	}
	if err := writeJSON(cmd, response); err != nil {
		return err
	}
	if runErr != nil {
		return &commandError{err: runErr, exitCode: exitCode, emitted: true}
	}
	return nil
}

func buildLiveObserver(descs []live.Descriptor) live.Observer {
	if len(descs) == 0 {
		return live.NopObserver{}
	}
	children := make([]live.Observer, 0, len(descs))
	for _, descriptor := range descs {
		children = append(children, live.NewHTTPObserver(descriptor.BaseURL, descriptor.Token))
	}
	return live.NewFanoutObserver(children...)
}

func envInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func envDuration(name string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
