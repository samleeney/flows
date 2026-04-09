package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "flow",
		Short: "Declarative agent orchestration tool",
	}

	root.AddCommand(
		newRunCmd(),
		newValidateCmd(),
		newVizCmd(),
	)

	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
