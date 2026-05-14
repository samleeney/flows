// Package parser reads a flow markdown file and produces a model.Flow.
package parser

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"

	"github.com/samleeney/flows/pkg/model"
	"gopkg.in/yaml.v3"
)

// Parse reads markdown source bytes and returns a Flow.
func Parse(src []byte) (*model.Flow, error) {
	body, frontmatter, err := extractFrontmatter(src)
	if err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}

	flow := &model.Flow{}
	if err := parseFrontmatter(frontmatter, flow); err != nil {
		return nil, fmt.Errorf("frontmatter: %w", err)
	}

	agents, err := parseAgents(body)
	if err != nil {
		return nil, err
	}
	flow.Agents = agents

	return flow, nil
}

// ParseFile is a convenience that reads a file and parses it.
func ParseFile(path string) (*model.Flow, error) {
	src, err := readFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(src)
}

// extractFrontmatter splits YAML frontmatter (between --- delimiters) from the
// body. Returns body and frontmatter separately.
func extractFrontmatter(src []byte) (body []byte, frontmatter []byte, err error) {
	s := string(src)
	if !strings.HasPrefix(s, "---") {
		return src, nil, nil
	}

	end := strings.Index(s[3:], "\n---")
	if end == -1 {
		return nil, nil, fmt.Errorf("unterminated frontmatter: no closing ---")
	}

	// frontmatter is between first --- and second ---
	fm := s[3 : end+3]
	// body starts after the closing --- and its newline
	rest := s[end+3+4:] // skip "\n---"
	if len(rest) > 0 && rest[0] == '\n' {
		rest = rest[1:]
	}

	return []byte(rest), []byte(fm), nil
}

// frontmatterData is the raw YAML structure of the frontmatter.
type frontmatterData struct {
	Name           string         `yaml:"name"`
	Description    string         `yaml:"description"`
	ExternalInputs []string       `yaml:"external_inputs"`
	Defaults       model.Defaults `yaml:"defaults"`
}

func parseFrontmatter(data []byte, flow *model.Flow) error {
	if data == nil {
		return fmt.Errorf("missing frontmatter")
	}

	var fm frontmatterData
	if err := yaml.Unmarshal(data, &fm); err != nil {
		return fmt.Errorf("parsing YAML: %w", err)
	}

	if fm.Name == "" {
		return fmt.Errorf("missing required field: name")
	}

	flow.Name = fm.Name
	flow.Description = fm.Description
	flow.ExternalInputs = fm.ExternalInputs
	flow.Defaults = fm.Defaults
	return nil
}

// parseAgents splits the body on ## headings and extracts each agent section
// using a line-based scanner. Prose content is preserved as raw bytes so
// markdown formatting (lists, emphasis, links, etc.) is never lost.
func parseAgents(body []byte) ([]model.Agent, error) {
	sections := splitSections(body)
	agents := make([]model.Agent, 0, len(sections))
	for _, sec := range sections {
		agent, err := parseSection(sec)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", sec.name, err)
		}
		agents = append(agents, agent)
	}
	return agents, nil
}

type section struct {
	name string
	body string // raw markdown between the ## heading and the next ## heading
}

// splitSections walks the body line-by-line and groups lines under each
// top-level ## heading. A line starting with ``` begins a fenced code block;
// any ## lines inside such a block are treated as content, not headings.
func splitSections(body []byte) []section {
	var sections []section
	var current *section
	var buf bytes.Buffer
	inFence := false
	fenceMark := ""

	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()

		// Track fenced code blocks so we don't mistake ## inside code for a heading
		if !inFence {
			if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
				inFence = true
				fenceMark = line[:3]
			}
		} else if strings.HasPrefix(line, fenceMark) {
			inFence = false
			fenceMark = ""
		}

		if !inFence && strings.HasPrefix(line, "## ") {
			// End previous section
			if current != nil {
				current.body = buf.String()
				sections = append(sections, *current)
				buf.Reset()
			}
			name := strings.TrimSpace(strings.TrimPrefix(line, "## "))
			current = &section{name: name}
			continue
		}

		if current != nil {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}

	if current != nil {
		current.body = buf.String()
		sections = append(sections, *current)
	}

	return sections
}

// agentConfig is the raw YAML config block for an agent.
type agentConfig struct {
	Position       [2]int                 `yaml:"position"`
	Inputs         map[string]model.Input `yaml:"inputs"`
	Start          []model.Condition      `yaml:"start"`
	PromptExecutor string                 `yaml:"prompt_executor"`
	Model          string                 `yaml:"model"`
	Temperature    float64                `yaml:"temperature"`
	OnError        string                 `yaml:"on_error"`
	OnExhaustion   string                 `yaml:"on_exhaustion"`
}

// parseSection extracts the first ```yaml code block as config, then determines
// whether the rest is prose (prompt) or a ```lang code block (function).
func parseSection(sec section) (model.Agent, error) {
	agent := model.Agent{
		Name:   sec.name,
		Inputs: make(map[string]model.Input),
	}

	yamlContent, afterYAML, ok := extractFirstCodeBlock(sec.body, "yaml")
	if !ok {
		return agent, fmt.Errorf("missing yaml config block")
	}

	var cfg agentConfig
	if err := yaml.Unmarshal([]byte(yamlContent), &cfg); err != nil {
		return agent, fmt.Errorf("parsing config YAML: %w", err)
	}
	agent.Position = cfg.Position
	agent.Inputs = cfg.Inputs
	if agent.Inputs == nil {
		agent.Inputs = make(map[string]model.Input)
	}
	agent.Start = cfg.Start
	agent.PromptExecutor = cfg.PromptExecutor
	agent.Model = cfg.Model
	agent.Temperature = cfg.Temperature
	agent.OnError = cfg.OnError
	agent.OnExhaustion = cfg.OnExhaustion

	// Look at what follows the YAML block. Skip leading blank lines.
	// If the next non-blank content is a fenced code block in another language,
	// it's a function node. Otherwise, the remaining text is the prompt.
	trimmedStart := strings.TrimLeft(afterYAML, "\n \t")
	if strings.HasPrefix(trimmedStart, "```") || strings.HasPrefix(trimmedStart, "~~~") {
		lang, code, _, ok := extractFirstCodeBlockAny(afterYAML)
		if ok && lang != "" && lang != "yaml" {
			agent.NodeType = model.FunctionNode
			agent.Language = lang
			agent.Content = code
			return agent, nil
		}
	}

	agent.NodeType = model.PromptNode
	agent.Content = strings.TrimSpace(afterYAML)
	return agent, nil
}

// extractFirstCodeBlock finds the first fenced code block with the given
// language tag and returns its content plus the text following the closing
// fence. Returns false if no such block exists.
func extractFirstCodeBlock(body, wantLang string) (content, after string, found bool) {
	lines := strings.SplitAfter(body, "\n")
	var codeBuf bytes.Buffer
	inBlock := false
	var fenceMark string

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\n")
		if !inBlock {
			if isFenceStart(trimmed, wantLang) {
				inBlock = true
				if strings.HasPrefix(trimmed, "```") {
					fenceMark = "```"
				} else {
					fenceMark = "~~~"
				}
				continue
			}
		} else {
			if strings.HasPrefix(trimmed, fenceMark) {
				// End of block
				after := strings.Join(lines[i+1:], "")
				return strings.TrimRight(codeBuf.String(), "\n"), after, true
			}
			codeBuf.WriteString(line)
		}
	}

	return "", "", false
}

// extractFirstCodeBlockAny finds the first fenced code block of any language.
func extractFirstCodeBlockAny(body string) (lang, content, after string, found bool) {
	lines := strings.SplitAfter(body, "\n")
	var codeBuf bytes.Buffer
	inBlock := false
	var fenceMark string

	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\n")
		if !inBlock {
			if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
				inBlock = true
				if strings.HasPrefix(trimmed, "```") {
					fenceMark = "```"
					lang = strings.TrimSpace(trimmed[3:])
				} else {
					fenceMark = "~~~"
					lang = strings.TrimSpace(trimmed[3:])
				}
				continue
			}
		} else {
			if strings.HasPrefix(trimmed, fenceMark) {
				after := strings.Join(lines[i+1:], "")
				return lang, strings.TrimRight(codeBuf.String(), "\n"), after, true
			}
			codeBuf.WriteString(line)
		}
	}

	return "", "", "", false
}

// isFenceStart reports whether a line opens a code block with the given lang.
func isFenceStart(line, wantLang string) bool {
	var rest string
	if strings.HasPrefix(line, "```") {
		rest = line[3:]
	} else if strings.HasPrefix(line, "~~~") {
		rest = line[3:]
	} else {
		return false
	}
	return strings.TrimSpace(rest) == wantLang
}
