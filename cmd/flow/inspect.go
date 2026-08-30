package main

import (
	"fmt"

	"github.com/samleeney/flows/pkg/clioutput"
	"github.com/spf13/cobra"
)

func newInspectCmd() *cobra.Command {
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "inspect <file>",
		Short: "Inspect a parsed and validated flow",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, source, diagnostics, err := parseAndValidate(cmd, args[0])
			if err != nil {
				if jsonOutput {
					return failJSON(cmd, "inspect", "invalid_flow", fmt.Sprintf("%s failed for %s", diagnostics[0].Phase, source), diagnostics, exitInvalid)
				}
				return fmt.Errorf("%s failed (%s): %w", diagnostics[0].Phase, source, err)
			}
			inspection := clioutput.FlowInspection(source, flow)
			if jsonOutput {
				return writeJSON(cmd, clioutput.Response{SchemaVersion: clioutput.SchemaVersion, Command: "inspect", OK: true, Flow: &inspection})
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Flow: %s\nDescription: %s\nExternal inputs: %v\nBlocks: %d\n", inspection.Name, inspection.Description, inspection.ExternalInputs, inspection.BlockCount)
			for _, block := range inspection.Blocks {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s (%s)\n", block.Name, block.Kind)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit one versioned JSON result")
	return cmd
}
