package runtime

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// BashExecutor runs bash code blocks. Inputs are passed as environment
// variables, so the code can reference them as $name.
type BashExecutor struct{}

func (b *BashExecutor) Language() string { return "bash" }

func (b *BashExecutor) Execute(ctx context.Context, content string, inputs map[string]string) (string, error) {
	cmd := exec.CommandContext(ctx, "bash", "-c", content)
	cmd.Env = append(cmd.Environ(), envFromInputs(inputs)...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("bash execution failed: %w\noutput: %s", err, string(out))
	}

	return strings.TrimSpace(string(out)), nil
}

func envFromInputs(inputs map[string]string) []string {
	env := make([]string, 0, len(inputs))
	for k, v := range inputs {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return env
}
