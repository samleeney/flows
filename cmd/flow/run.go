package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/runtime"
	"github.com/samleeney/flows/pkg/validator"
	"github.com/spf13/cobra"
)

func newRunCmd() *cobra.Command {
	var (
		inputs  []string
		verbose bool
		dryRun  bool
		outDir  string
	)

	cmd := &cobra.Command{
		Use:   "run <file>",
		Short: "Execute a flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			if err := validator.Validate(flow); err != nil {
				return fmt.Errorf("validation failed:\n%w", err)
			}

			// Parse --input flags
			externalInputs := make(map[string]string)
			for _, kv := range inputs {
				parts := strings.SplitN(kv, "=", 2)
				if len(parts) != 2 {
					return fmt.Errorf("invalid input format %q, expected name=value", kv)
				}
				name, value := parts[0], parts[1]

				// @filepath means read from file
				if strings.HasPrefix(value, "@") {
					data, err := os.ReadFile(value[1:])
					if err != nil {
						return fmt.Errorf("reading input file for %q: %w", name, err)
					}
					value = string(data)
				}
				externalInputs[name] = value
			}

			if dryRun {
				fmt.Printf("Flow: %s\n", flow.Name)
				fmt.Printf("Agents: %d\n", len(flow.Agents))
				fmt.Printf("External inputs: %v\n", flow.ExternalInputs)
				fmt.Println("Dry run — not executing.")
				return nil
			}

			// Build executor registry
			// TODO: add real LLM prompt executor
			prompt := &stubPromptExecutor{}
			registry := runtime.NewExecutorRegistry(prompt, &runtime.BashExecutor{})

			opts := runtime.RunOptions{
				ExternalInputs: externalInputs,
				Verbose:        verbose,
			}

			if verbose {
				opts.OnAgentStart = func(name string, iter int) {
					fmt.Printf("[%s] iteration %d starting...\n", name, iter)
				}
				opts.OnAgentDone = func(name string, iter int, output string, err error) {
					if err != nil {
						fmt.Printf("[%s] iteration %d FAILED: %v\n", name, iter, err)
					} else {
						preview := output
						if len(preview) > 100 {
							preview = preview[:100] + "..."
						}
						fmt.Printf("[%s] iteration %d done: %s\n", name, iter, preview)
					}
				}
			}

			result, err := runtime.Run(context.Background(), flow, registry, opts)
			if err != nil {
				return fmt.Errorf("execution failed: %w", err)
			}

			// Write outputs
			if outDir != "" {
				if err := os.MkdirAll(outDir, 0o755); err != nil {
					return err
				}
				for name, output := range result.Outputs {
					path := filepath.Join(outDir, name+".txt")
					if err := os.WriteFile(path, []byte(output), 0o644); err != nil {
						return fmt.Errorf("writing output for %q: %w", name, err)
					}
				}
				fmt.Printf("Outputs written to %s/\n", outDir)
			} else {
				for name, output := range result.Outputs {
					fmt.Printf("=== %s ===\n%s\n\n", name, output)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringArrayVar(&inputs, "input", nil, "External input as name=value (repeatable)")
	cmd.Flags().BoolVar(&verbose, "verbose", false, "Print agent execution details")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and show plan without running")
	cmd.Flags().StringVar(&outDir, "output", "", "Directory to write agent outputs")

	return cmd
}

// stubPromptExecutor is a placeholder until real LLM integration is added.
type stubPromptExecutor struct{}

func (s *stubPromptExecutor) Execute(_ context.Context, content string, inputs map[string]string) (string, error) {
	return fmt.Sprintf("[stub] would send prompt (%d chars) with %d inputs to LLM", len(content), len(inputs)), nil
}
