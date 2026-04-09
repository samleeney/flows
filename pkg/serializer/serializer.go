// Package serializer writes a model.Flow back to markdown format.
package serializer

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/samleeney/flows/pkg/model"
	"gopkg.in/yaml.v3"
)

// Serialize writes a Flow to markdown bytes.
func Serialize(flow *model.Flow) ([]byte, error) {
	var buf bytes.Buffer

	if err := writeFrontmatter(&buf, flow); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}

	for i, agent := range flow.Agents {
		if err := writeAgent(&buf, &agent); err != nil {
			return nil, fmt.Errorf("agent %q: %w", agent.Name, err)
		}
		if i < len(flow.Agents)-1 {
			buf.WriteByte('\n')
		}
	}

	return buf.Bytes(), nil
}

// frontmatterData mirrors the YAML structure for serialization.
type frontmatterData struct {
	Name           string          `yaml:"name"`
	Description    string          `yaml:"description,omitempty"`
	ExternalInputs []string        `yaml:"external_inputs"`
	Defaults       *model.Defaults `yaml:"defaults,omitempty"`
}

func writeFrontmatter(buf *bytes.Buffer, flow *model.Flow) error {
	fm := frontmatterData{
		Name:           flow.Name,
		Description:    flow.Description,
		ExternalInputs: flow.ExternalInputs,
	}
	if flow.Defaults.Model != "" || flow.Defaults.Temperature != 0 {
		fm.Defaults = &flow.Defaults
	}

	data, err := yaml.Marshal(&fm)
	if err != nil {
		return err
	}

	buf.WriteString("---\n")
	buf.Write(data)
	buf.WriteString("---\n\n")
	return nil
}

// agentConfig is the YAML config block for an agent.
type agentConfig struct {
	Position     [2]int                 `yaml:"position,omitempty,flow"`
	Inputs       map[string]model.Input `yaml:"inputs,omitempty"`
	Start        []model.Condition      `yaml:"start,omitempty"`
	Model        string                 `yaml:"model,omitempty"`
	Temperature  float64                `yaml:"temperature,omitempty"`
	OnError      string                 `yaml:"on_error,omitempty"`
	OnExhaustion string                 `yaml:"on_exhaustion,omitempty"`
}

func writeAgent(buf *bytes.Buffer, agent *model.Agent) error {
	// Heading
	fmt.Fprintf(buf, "## %s\n\n", agent.Name)

	// YAML config block
	cfg := agentConfig{
		Position:     agent.Position,
		Inputs:       agent.Inputs,
		Start:        agent.Start,
		Model:        agent.Model,
		Temperature:  agent.Temperature,
		OnError:      agent.OnError,
		OnExhaustion: agent.OnExhaustion,
	}

	// Only include inputs if non-empty
	if len(cfg.Inputs) == 0 {
		cfg.Inputs = nil
	}

	data, err := yaml.Marshal(&cfg)
	if err != nil {
		return err
	}

	buf.WriteString("```yaml\n")
	buf.Write(data)
	buf.WriteString("```\n\n")

	// Content
	if agent.Content != "" {
		switch agent.NodeType {
		case model.FunctionNode:
			lang := agent.Language
			if lang == "" {
				lang = "bash"
			}
			fmt.Fprintf(buf, "```%s\n%s\n```\n", lang, agent.Content)
		case model.PromptNode:
			buf.WriteString(agent.Content)
			buf.WriteByte('\n')
		}
	}

	return nil
}

// SerializeToString is a convenience that returns a string.
func SerializeToString(flow *model.Flow) (string, error) {
	data, err := Serialize(flow)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(data), "\n") + "\n", nil
}
