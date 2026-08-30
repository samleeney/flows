package main

import (
	"fmt"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/samleeney/flows/pkg/viz"
	"github.com/spf13/cobra"
)

func newVizCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "viz <file>",
		Short: "Output flow graph as a Mermaid diagram",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, source, diagnostics, err := parseFlowSource(cmd, args[0])
			if err != nil {
				if jsonOutput {
					return failJSON(cmd, "viz", "invalid_flow", fmt.Sprintf("%s failed for %s", diagnostics[0].Phase, source), diagnostics, exitInvalid)
				}
				if source == "<stdin>" {
					return fmt.Errorf("parse error (%s): %w", source, err)
				}
				return fmt.Errorf("parse error: %w", err)
			}
			mermaid := viz.Mermaid(flow)
			if jsonOutput {
				summary := clioutput.FlowSummary(source, flow)
				return writeJSON(cmd, clioutput.Response{SchemaVersion: clioutput.SchemaVersion, Command: "viz", OK: true, Flow: &summary, Mermaid: mermaid})
			}
			fmt.Fprint(cmd.OutOrStdout(), mermaid)
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit one versioned JSON result")
	return cmd
}
