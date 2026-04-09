package runtime

import (
	"context"
	"testing"
)

func TestPythonExecutorSimple(t *testing.T) {
	exec := &PythonExecutor{}
	out, err := exec.Execute(context.Background(), `output = "hello from python"`, map[string]string{})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "hello from python" {
		t.Errorf("output = %q, want %q", out, "hello from python")
	}
}

func TestPythonExecutorWithInputs(t *testing.T) {
	exec := &PythonExecutor{}
	out, err := exec.Execute(context.Background(),
		`output = f"Hello, {name}!"`,
		map[string]string{"name": "World"},
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "Hello, World!" {
		t.Errorf("output = %q, want %q", out, "Hello, World!")
	}
}

func TestPythonExecutorComputation(t *testing.T) {
	exec := &PythonExecutor{}
	out, err := exec.Execute(context.Background(),
		`
x = int(a) + int(b)
output = str(x)
`,
		map[string]string{"a": "10", "b": "20"},
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	if out != "30" {
		t.Errorf("output = %q, want %q", out, "30")
	}
}

func TestPythonExecutorError(t *testing.T) {
	exec := &PythonExecutor{}
	_, err := exec.Execute(context.Background(), `raise ValueError("bad")`, map[string]string{})
	if err == nil {
		t.Fatal("expected error from python")
	}
}
