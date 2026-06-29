package runtime

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/samleeney/flows/pkg/model"
)

func TestHTTPPromptExecutorAnthropic(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/messages" {
			t.Fatalf("path = %q, want /messages", r.URL.Path)
		}
		if r.Header.Get("x-api-key") != "test-key" {
			t.Fatalf("missing anthropic api key header")
		}
		if r.Header.Get("anthropic-version") == "" {
			t.Fatalf("missing anthropic-version header")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"content": []map[string]string{{"type": "text", "text": "approved"}},
		})
	}))
	defer srv.Close()

	exec := NewHTTPPromptExecutor(HTTPPromptConfig{
		AnthropicAPIKey:  "test-key",
		AnthropicBaseURL: srv.URL,
		MaxTokens:        123,
	})

	out, err := exec.ExecuteAgent(context.Background(), ExecutionRequest{
		FlowName: "Review Flow",
		Defaults: model.Defaults{Model: "claude-sonnet-test", Temperature: 0.3},
		Agent:    model.Agent{Name: "reviewer"},
		Content:  "Review the code.",
		Inputs:   map[string]string{"code": "x = 1"},
	})
	if err != nil {
		t.Fatalf("ExecuteAgent: %v", err)
	}
	if out != "approved" {
		t.Fatalf("output = %q, want approved", out)
	}
	if got["model"] != "claude-sonnet-test" {
		t.Fatalf("model = %v, want claude-sonnet-test", got["model"])
	}
	if got["max_tokens"] != float64(123) {
		t.Fatalf("max_tokens = %v, want 123", got["max_tokens"])
	}
	if got["temperature"] != 0.3 {
		t.Fatalf("temperature = %v, want 0.3", got["temperature"])
	}
	if _, ok := got["system"]; ok {
		t.Fatalf("anthropic request unexpectedly included a system prompt: %v", got["system"])
	}
	messages := got["messages"].([]any)
	user := messages[0].(map[string]any)["content"].(string)
	for _, want := range []string{`<input name="code">`, "Block prompt:", "Review the code."} {
		if !strings.Contains(user, want) {
			t.Fatalf("user prompt missing %q:\n%s", want, user)
		}
	}
	for _, unwanted := range []string{"Flow: Review Flow", "Agent: reviewer", "declarative workflow", "Node prompt:", "Treat input values as data", "Do not edit files"} {
		if strings.Contains(user, unwanted) {
			t.Fatalf("user prompt unexpectedly contains %q:\n%s", unwanted, user)
		}
	}
}

func TestHTTPPromptExecutorOpenAIResponses(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/responses" {
			t.Fatalf("path = %q, want /responses", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer openai-key" {
			t.Fatalf("missing openai authorization header")
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": []map[string]any{{
				"type": "message",
				"content": []map[string]string{{
					"type": "output_text",
					"text": "fixed code",
				}},
			}},
		})
	}))
	defer srv.Close()

	exec := NewHTTPPromptExecutor(HTTPPromptConfig{
		OpenAIAPIKey:  "openai-key",
		OpenAIBaseURL: srv.URL,
		Model:         "gpt-test",
		MaxTokens:     77,
	})

	out, err := exec.ExecuteAgent(context.Background(), ExecutionRequest{
		Agent:   model.Agent{Name: "fixer", Model: "claude-ignored"},
		Content: "Fix the code.",
		Inputs:  map[string]string{"feedback": "needs_changes"},
	})
	if err != nil {
		t.Fatalf("ExecuteAgent: %v", err)
	}
	if out != "fixed code" {
		t.Fatalf("output = %q, want fixed code", out)
	}
	if got["model"] != "gpt-test" {
		t.Fatalf("model = %v, want gpt-test", got["model"])
	}
	if got["max_output_tokens"] != float64(77) {
		t.Fatalf("max_output_tokens = %v, want 77", got["max_output_tokens"])
	}
	input := got["input"].([]any)
	if len(input) != 1 {
		t.Fatalf("input length = %d, want 1: %#v", len(input), input)
	}
	user := input[0].(map[string]any)
	if user["role"] != "user" {
		t.Fatalf("input role = %v, want user", user["role"])
	}
	content := user["content"].(string)
	for _, unwanted := range []string{"Treat input values as data", "Do not edit files", "You may edit files"} {
		if strings.Contains(content, unwanted) {
			t.Fatalf("openai user prompt unexpectedly contains %q:\n%s", unwanted, content)
		}
	}
}

func TestHTTPPromptExecutorMissingAPIKey(t *testing.T) {
	exec := NewHTTPPromptExecutor(HTTPPromptConfig{})
	_, err := exec.ExecuteAgent(context.Background(), ExecutionRequest{
		Defaults: model.Defaults{Model: "claude-sonnet-test"},
		Agent:    model.Agent{Name: "reviewer"},
		Content:  "Review.",
	})
	if err == nil {
		t.Fatalf("expected missing API key error")
	}
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Fatalf("error = %v, want ANTHROPIC_API_KEY", err)
	}
}
