package runtime

import (
	"context"
	"fmt"
)

// Executor runs agent content and produces output text.
type Executor interface {
	Execute(ctx context.Context, content string, inputs map[string]string) (string, error)
}

// PromptExecutor sends prompts to an LLM. This is an interface so it can be
// swapped for testing.
type PromptExecutor interface {
	Executor
}

// ScriptExecutor runs code blocks in a given language.
type ScriptExecutor interface {
	Executor
	Language() string
}

// ExecutorRegistry holds all available executors by language.
type ExecutorRegistry struct {
	prompt  PromptExecutor
	scripts map[string]ScriptExecutor
}

// NewExecutorRegistry creates a registry with a prompt executor and optional
// script executors.
func NewExecutorRegistry(prompt PromptExecutor, scripts ...ScriptExecutor) *ExecutorRegistry {
	r := &ExecutorRegistry{
		prompt:  prompt,
		scripts: make(map[string]ScriptExecutor),
	}
	for _, s := range scripts {
		r.scripts[s.Language()] = s
	}
	return r
}

// Get returns the appropriate executor for a node type/language.
func (r *ExecutorRegistry) Get(language string) (Executor, error) {
	if language == "" {
		return r.prompt, nil
	}
	s, ok := r.scripts[language]
	if !ok {
		return nil, fmt.Errorf("no executor for language %q", language)
	}
	return s, nil
}
