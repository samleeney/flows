package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

// BashExecutor runs bash code blocks.
type BashExecutor struct{}

func (b *BashExecutor) Language() string { return "bash" }

func (b *BashExecutor) Execute(ctx context.Context, content string, inputs map[string]string) (string, error) {
	inputJSON, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("marshaling inputs: %w", err)
	}

	// Wrap the code: read inputs from stdin JSON, expose as variables, run code
	wrapper := fmt.Sprintf(`
set -euo pipefail
eval "$(echo '%s' | python3 -c "
import json, sys
for k, v in json.load(sys.stdin).items():
    print(f'{k}={json.dumps(v)}')" 2>/dev/null || true)"
%s
`, strings.ReplaceAll(string(inputJSON), "'", "'\\''"), content)

	cmd := exec.CommandContext(ctx, "bash", "-c", wrapper)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bash execution failed: %w\noutput: %s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}
