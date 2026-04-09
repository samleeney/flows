package main

import (
	"fmt"

	"github.com/samleeney/flows/pkg/parser"
	"github.com/samleeney/flows/pkg/viz"
	"github.com/spf13/cobra"
)

func newVizCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "viz <file>",
		Short: "Output flow graph as a Mermaid diagram",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			flow, err := parser.ParseFile(args[0])
			if err != nil {
				return fmt.Errorf("parse error: %w", err)
			}

			fmt.Print(viz.Mermaid(flow))
			return nil
		},
	}
}
