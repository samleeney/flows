package editor

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/samleeney/flows/pkg/live"
	"github.com/samleeney/flows/pkg/runtime"
)

const tinyBashFlow = `---
name: Live Test Flow
description: tiny bash flow
external_inputs:
  - name
---

## hello

` + "```yaml" + `
position: [0, 0]
inputs:
  name: { from: external }
start:
  - always: { max_runs: 1 }
` + "```" + `

` + "```bash" + `
echo "hello, $name"
` + "```" + `

## shout

` + "```yaml" + `
position: [1, 0]
inputs:
  msg: { from: hello }
start:
  - when: hello
` + "```" + `

` + "```bash" + `
echo "$msg" | tr '[:lower:]' '[:upper:]'
` + "```" + `
`

func writeTestFlow(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "live_test.md")
	if err := os.WriteFile(path, []byte(tinyBashFlow), 0o644); err != nil {
		t.Fatalf("write flow: %v", err)
	}
	return path
}

// startTestEditor stands up a Server wrapped in httptest.NewServer with live
// endpoints enabled. Returns the server, the test HTTP server, and the
// canonical path / token / flow_key used.
func startTestEditor(t *testing.T, flowPath string) (*Server, *httptest.Server, string, string) {
	t.Helper()
	canonical, err := live.CanonicalFlowPath(flowPath)
	if err != nil {
		t.Fatalf("canonical: %v", err)
	}
	flowKey := live.FlowKey(canonical)
	token, err := live.NewToken()
	if err != nil {
		t.Fatalf("token: %v", err)
	}
	srv, err := NewServer(NewServerOptions{
		FilePath:      flowPath,
		CanonicalPath: canonical,
		FlowKey:       flowKey,
		Token:         token,
	})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	httpSrv := httptest.NewServer(srv.Handler())
	return srv, httpSrv, token, flowKey
}

// runFlow executes the tiny bash flow against the given test editor URL.
func runFlow(t *testing.T, flowPath, baseURL, token, flowKey string) {
	t.Helper()

	// Reparse via the runtime path so we exercise the same code as `flow run`.
	srvForFlow, err := NewServer(NewServerOptions{FilePath: flowPath})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	defer srvForFlow.Close()
	flow := jsonToFlow(toFlowJSON(srvForFlow))

	registry := runtime.NewExecutorRegistry(nil, &runtime.BashExecutor{}, &runtime.PythonExecutor{})
	observer := live.NewHTTPObserver(baseURL, token)
	defer observer.Close()

	_, err = runtime.Run(context.Background(), flow, registry, runtime.RunOptions{
		ExternalInputs: map[string]string{"name": "world"},
		FlowKey:        flowKey,
		Observer:       observer,
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
}

// toFlowJSON pulls the parsed Flow out of a Server (used to roundtrip into the
// runtime model).
func toFlowJSON(s *Server) *FlowJSON {
	s.mu.RLock()
	defer s.mu.RUnlock()
	fj := flowToJSON(s.flow)
	return &fj
}

func TestLiveIntegrationSingleRun(t *testing.T) {
	flowPath := writeTestFlow(t)
	srv, httpSrv, token, flowKey := startTestEditor(t, flowPath)
	defer httpSrv.Close()
	defer srv.Close()

	runFlow(t, flowPath, httpSrv.URL, token, flowKey)

	snap := srv.RunSnapshot()
	if len(snap.Runs) != 1 {
		t.Fatalf("expected 1 run in snapshot, got %d", len(snap.Runs))
	}
	rec := snap.Runs[0]
	if rec.FinishedAt == nil {
		t.Fatalf("run not marked finished")
	}
	if rec.OK == nil || !*rec.OK {
		t.Fatalf("run.OK = %v, want true", rec.OK)
	}
	if rec.LastSeq == 0 {
		t.Fatalf("last_seq = 0, want > 0")
	}
	for _, name := range []string{"hello", "shout"} {
		ag, ok := rec.Agents[name]
		if !ok {
			t.Errorf("agent %q missing from run record", name)
			continue
		}
		if ag.Status != "done" {
			t.Errorf("agent %q status = %q, want done", name, ag.Status)
		}
		if ag.Iter < 1 {
			t.Errorf("agent %q iter = %d, want >= 1", name, ag.Iter)
		}
		if ag.OutputPreview == "" {
			t.Errorf("agent %q output_preview empty", name)
		}
	}
}

func TestLiveIntegrationConcurrentRuns(t *testing.T) {
	flowPath := writeTestFlow(t)
	srv, httpSrv, token, flowKey := startTestEditor(t, flowPath)
	defer httpSrv.Close()
	defer srv.Close()

	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			runFlow(t, flowPath, httpSrv.URL, token, flowKey)
		}()
	}
	wg.Wait()

	snap := srv.RunSnapshot()
	if len(snap.Runs) != 2 {
		t.Fatalf("expected 2 distinct runs in snapshot, got %d", len(snap.Runs))
	}
	seen := make(map[string]bool)
	for _, rec := range snap.Runs {
		if seen[rec.RunID] {
			t.Errorf("duplicate run_id %q", rec.RunID)
		}
		seen[rec.RunID] = true
		if rec.FinishedAt == nil || rec.OK == nil || !*rec.OK {
			t.Errorf("run %q did not finish OK", rec.RunID)
		}
		// Each run should still have its own complete agent set.
		if _, ok := rec.Agents["hello"]; !ok {
			t.Errorf("run %q missing hello agent", rec.RunID)
		}
		if _, ok := rec.Agents["shout"]; !ok {
			t.Errorf("run %q missing shout agent", rec.RunID)
		}
	}
}
