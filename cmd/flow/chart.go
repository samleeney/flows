package main

import (
	"fmt"
	"log"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/samleeney/flows/pkg/editor"
	"github.com/spf13/cobra"
)

func newChartCmd() *cobra.Command {
	var (
		port   int
		noOpen bool
		uiDir  string
	)

	cmd := &cobra.Command{
		Use:   "chart <file>",
		Short: "Open visual editor in browser",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var uiFS http.FileSystem
			if uiDir != "" {
				uiFS = http.Dir(uiDir)
			} else {
				uiFS = embeddedUI()
			}

			srv, err := editor.NewServer(args[0], uiFS)
			if err != nil {
				return fmt.Errorf("creating editor: %w", err)
			}
			defer srv.Close()

			if err := srv.StartFileWatcher(); err != nil {
				log.Printf("warning: file watcher failed: %v", err)
			}

			addr := fmt.Sprintf("localhost:%d", port)
			url := fmt.Sprintf("http://%s", addr)

			fmt.Printf("Flow editor running at %s\n", url)
			fmt.Println("Press Ctrl+C to stop.")

			if !noOpen {
				go openBrowser(url)
			}

			return http.ListenAndServe(addr, srv.Handler())
		},
	}

	cmd.Flags().IntVar(&port, "port", 8420, "Port for the editor server")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Don't auto-open the browser")
	cmd.Flags().StringVar(&uiDir, "ui-dir", "", "Path to built frontend assets (default: embedded)")

	return cmd
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	cmd.Run()
}
