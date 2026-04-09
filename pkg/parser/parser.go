// Package parser reads a flow markdown file and produces a model.Flow.
package parser

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/samleeney/flows/pkg/model"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
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

// parseAgents walks the goldmark AST to extract agent sections.
func parseAgents(body []byte) ([]model.Agent, error) {
	md := goldmark.New()
	reader := text.NewReader(body)
	doc := md.Parser().Parse(reader)

	var agents []model.Agent
	var currentName string
	var sectionNodes []ast.Node

	// Collect nodes grouped by ## headings.
	for node := doc.FirstChild(); node != nil; node = node.NextSibling() {
		heading, isHeading := node.(*ast.Heading)
		if isHeading && heading.Level == 2 {
			// Process previous section if any
			if currentName != "" {
				agent, err := buildAgent(currentName, sectionNodes, body)
				if err != nil {
					return nil, fmt.Errorf("agent %q: %w", currentName, err)
				}
				agents = append(agents, agent)
			}
			currentName = extractHeadingText(heading, body)
			sectionNodes = nil
			continue
		}
		if currentName != "" {
			sectionNodes = append(sectionNodes, node)
		}
	}

	// Process the last section
	if currentName != "" {
		agent, err := buildAgent(currentName, sectionNodes, body)
		if err != nil {
			return nil, fmt.Errorf("agent %q: %w", currentName, err)
		}
		agents = append(agents, agent)
	}

	return agents, nil
}

func extractHeadingText(heading *ast.Heading, src []byte) string {
	var buf bytes.Buffer
	for child := heading.FirstChild(); child != nil; child = child.NextSibling() {
		if t, ok := child.(*ast.Text); ok {
			buf.Write(t.Segment.Value(src))
		}
	}
	return strings.TrimSpace(buf.String())
}

// agentConfig is the raw YAML config block for an agent.
type agentConfig struct {
	Position     [2]int                `yaml:"position"`
	Inputs       map[string]model.Input `yaml:"inputs"`
	Start        []model.Condition      `yaml:"start"`
	Model        string                `yaml:"model"`
	Temperature  float64               `yaml:"temperature"`
	OnError      string                `yaml:"on_error"`
	OnExhaustion string                `yaml:"on_exhaustion"`
}

// buildAgent constructs an Agent from the AST nodes in its section.
func buildAgent(name string, nodes []ast.Node, src []byte) (model.Agent, error) {
	agent := model.Agent{
		Name:   name,
		Inputs: make(map[string]model.Input),
	}

	yamlFound := false
	var contentParts []string

	for _, node := range nodes {
		cb, isFenced := node.(*ast.FencedCodeBlock)
		if !isFenced {
			// Collect non-code-block text as potential prompt content
			raw := extractNodeText(node, src)
			if raw != "" {
				contentParts = append(contentParts, raw)
			}
			continue
		}

		lang := string(cb.Language(src))
		code := extractCodeBlockContent(cb, src)

		if !yamlFound && lang == "yaml" {
			// First yaml block is the config
			var cfg agentConfig
			if err := yaml.Unmarshal([]byte(code), &cfg); err != nil {
				return agent, fmt.Errorf("parsing config YAML: %w", err)
			}
			agent.Position = cfg.Position
			agent.Inputs = cfg.Inputs
			if agent.Inputs == nil {
				agent.Inputs = make(map[string]model.Input)
			}
			agent.Start = cfg.Start
			agent.Model = cfg.Model
			agent.Temperature = cfg.Temperature
			agent.OnError = cfg.OnError
			agent.OnExhaustion = cfg.OnExhaustion
			yamlFound = true
			continue
		}

		// A non-yaml code block after the config → function node
		if yamlFound && lang != "yaml" && lang != "" {
			agent.NodeType = model.FunctionNode
			agent.Language = lang
			agent.Content = code
			return agent, nil
		}

		// Unexpected extra yaml or unlabeled code block — include as content
		contentParts = append(contentParts, code)
	}

	// If we get here, it's a prompt node
	agent.NodeType = model.PromptNode
	agent.Content = strings.TrimSpace(strings.Join(contentParts, "\n\n"))

	return agent, nil
}

// extractCodeBlockContent reads the text content of a fenced code block.
func extractCodeBlockContent(cb *ast.FencedCodeBlock, src []byte) string {
	var buf bytes.Buffer
	lines := cb.Lines()
	for i := 0; i < lines.Len(); i++ {
		seg := lines.At(i)
		buf.Write(seg.Value(src))
	}
	return strings.TrimRight(buf.String(), "\n")
}

// extractNodeText extracts raw source text for a node and its children.
func extractNodeText(node ast.Node, src []byte) string {
	var buf bytes.Buffer
	collectText(node, src, &buf)
	return strings.TrimSpace(buf.String())
}

func collectText(node ast.Node, src []byte, buf *bytes.Buffer) {
	if node.Type() == ast.TypeBlock {
		// For block nodes, reconstruct from lines
		if lb, ok := node.(interface{ Lines() *text.Segments }); ok {
			lines := lb.Lines()
			for i := 0; i < lines.Len(); i++ {
				seg := lines.At(i)
				buf.Write(seg.Value(src))
			}
		}
		// Also recurse into children for nested content
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			collectText(child, src, buf)
		}
	} else {
		// Inline nodes
		switch t := node.(type) {
		case *ast.Text:
			buf.Write(t.Segment.Value(src))
			if t.SoftLineBreak() {
				buf.WriteByte('\n')
			}
		default:
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				collectText(child, src, buf)
			}
		}
	}
}
