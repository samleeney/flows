package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"os/exec"
	"runtime"

	"github.com/samleeney/flows/pkg/editor"
	"github.com/samleeney/flows/pkg/live"
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
			filePath := args[0]

			canonical, err := live.CanonicalFlowPath(filePath)
			if err != nil {
				return fmt.Errorf("canonicalize: %w", err)
			}
			flowKey := live.FlowKey(canonical)

			token, err := live.NewToken()
			if err != nil {
				return fmt.Errorf("generate token: %w", err)
			}

			var uiFS http.FileSystem
			if uiDir != "" {
				uiFS = http.Dir(uiDir)
			} else {
				uiFS = embeddedUI()
			}

			srv, err := editor.NewServer(editor.NewServerOptions{
				FilePath:      filePath,
				CanonicalPath: canonical,
				FlowKey:       flowKey,
				Token:         token,
				UIFS:          uiFS,
			})
			if err != nil {
				return fmt.Errorf("creating editor: %w", err)
			}
			defer srv.Close()

			if err := srv.StartFileWatcher(); err != nil {
				log.Printf("warning: file watcher failed: %v", err)
			}

			addr := fmt.Sprintf("127.0.0.1:%d", port)
			ln, err := net.Listen("tcp", addr)
			if err != nil {
				return fmt.Errorf("bind %s: %w", addr, err)
			}

			boundAddr := ln.Addr().(*net.TCPAddr)
			baseURL := fmt.Sprintf("http://127.0.0.1:%d", boundAddr.Port)

			cleanup, _, err := live.RegisterDescriptor(live.Descriptor{
				BaseURL:       baseURL,
				Token:         token,
				FlowKey:       flowKey,
				CanonicalPath: canonical,
			})
			if err != nil {
				log.Printf("warning: live descriptor register failed: %v", err)
			} else {
				defer cleanup()
			}

			fmt.Printf("Flow editor running at %s\n", baseURL)
			fmt.Println("Press Ctrl+C to stop.")

			if !noOpen {
				go openBrowser(baseURL)
			}

			return srv.Serve(ln)
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
