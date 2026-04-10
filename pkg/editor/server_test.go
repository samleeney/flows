package editor

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func copyTestFile(t *testing.T) string {
	t.Helper()
	src := filepath.Join("..", "parser", "testdata", "code_review.md")
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("read test file: %v", err)
	}

	tmp := filepath.Join(t.TempDir(), "test_flow.md")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return tmp
}

func TestServerGetFlow(t *testing.T) {
	tmp := copyTestFile(t)
	srv, err := NewServer(tmp)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/flow", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var flowJSON FlowJSON
	if err := json.NewDecoder(w.Body).Decode(&flowJSON); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if flowJSON.Name != "Code Review Flow" {
		t.Errorf("name = %q, want %q", flowJSON.Name, "Code Review Flow")
	}
	if len(flowJSON.Agents) != 3 {
		t.Errorf("agents = %d, want 3", len(flowJSON.Agents))
	}

	// Check agent details
	reviewer := flowJSON.Agents[0]
	if reviewer.Name != "reviewer" {
		t.Errorf("agent[0].name = %q, want reviewer", reviewer.Name)
	}
	if reviewer.NodeType != "prompt" {
		t.Errorf("agent[0].nodeType = %q, want prompt", reviewer.NodeType)
	}
	if reviewer.Position != [2]int{0, 0} {
		t.Errorf("agent[0].position = %v, want [0,0]", reviewer.Position)
	}

	// Defaults should round-trip through the JSON layer.
	if flowJSON.Defaults.Model != "claude-sonnet-4-20250514" {
		t.Errorf("defaults.model = %q, want %q", flowJSON.Defaults.Model, "claude-sonnet-4-20250514")
	}
	if flowJSON.Defaults.Temperature != 0.3 {
		t.Errorf("defaults.temperature = %v, want 0.3", flowJSON.Defaults.Temperature)
	}
}

func TestServerRoundTripPreservesDefaults(t *testing.T) {
	tmp := copyTestFile(t)
	srv, err := NewServer(tmp)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	// GET the flow, then PUT it back unchanged. Defaults must survive.
	req := httptest.NewRequest(http.MethodGet, "/api/flow", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	body := w.Body.Bytes()
	putReq := httptest.NewRequest(http.MethodPut, "/api/flow", bytes.NewReader(body))
	putW := httptest.NewRecorder()
	srv.Handler().ServeHTTP(putW, putReq)
	if putW.Code != http.StatusOK {
		t.Fatalf("PUT failed: %d", putW.Code)
	}

	// Read the file back and verify defaults are still there
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Contains(data, []byte("claude-sonnet-4-20250514")) {
		t.Errorf("defaults.model was stripped during round-trip")
	}
	if !bytes.Contains(data, []byte("temperature: 0.3")) {
		t.Errorf("defaults.temperature was stripped during round-trip")
	}
}

func TestServerPutFlow(t *testing.T) {
	tmp := copyTestFile(t)
	srv, err := NewServer(tmp)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	// Modify the flow via PUT
	modified := FlowJSON{
		Name:           "Modified Flow",
		Description:    "Changed",
		ExternalInputs: []string{"code"},
		Agents: []AgentJSON{
			{
				Name:     "solo",
				Position: [2]int{0, 0},
				NodeType: "prompt",
				Inputs:   map[string]InputJSON{"code": {From: "external"}},
				Start:    []ConditionJSON{{Always: &AlwaysJSON{MaxRuns: 1}}},
				Content:  "Do work.",
			},
		},
	}

	body, _ := json.Marshal(modified)
	req := httptest.NewRequest(http.MethodPut, "/api/flow", bytes.NewReader(body))
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT status = %d, want 200", w.Code)
	}

	// Verify file was updated
	data, err := os.ReadFile(tmp)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if !bytes.Contains(data, []byte("Modified Flow")) {
		t.Error("file should contain modified flow name")
	}

	// Verify GET returns updated flow
	req2 := httptest.NewRequest(http.MethodGet, "/api/flow", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, req2)

	var result FlowJSON
	json.NewDecoder(w2.Body).Decode(&result)
	if result.Name != "Modified Flow" {
		t.Errorf("after PUT, name = %q, want %q", result.Name, "Modified Flow")
	}
}

func TestServerIndex(t *testing.T) {
	tmp := copyTestFile(t)
	srv, err := NewServer(tmp)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	defer srv.Close()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte("Flow Editor")) {
		t.Error("index should contain Flow Editor title")
	}
}
