package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// PythonExecutor runs python code blocks via python3 subprocess.
type PythonExecutor struct{}

func (p *PythonExecutor) Language() string { return "python" }

func (p *PythonExecutor) Execute(ctx context.Context, content string, inputs map[string]string) (string, error) {
	// Build a wrapper script that:
	// 1. Reads inputs from a JSON string injected as a variable
	// 2. Exposes each input as a local variable
	// 3. Runs the user code
	// 4. Prints the output variable as JSON to stdout
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("marshaling inputs: %w", err)
	}

	wrapper := fmt.Sprintf(`import json as _json, sys as _sys
_inputs = _json.loads(%q)
for _k, _v in _inputs.items():
    exec(f"{_k} = _json.loads({_json.dumps(_v)!r})" if _v.startswith(("{","[")) else f"{_k} = {_v!r}")
output = None
%s
if output is not None:
    if isinstance(output, str):
        print(output)
    else:
        print(_json.dumps(output))
`, string(inputJSON), content)

	pythonCmd := os.Getenv("FLOW_PYTHON_COMMAND")
	if pythonCmd == "" {
		pythonCmd = "python3"
	}
	cmd := exec.CommandContext(ctx, pythonCmd, "-c", wrapper)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("python execution failed: %w\noutput: %s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}
