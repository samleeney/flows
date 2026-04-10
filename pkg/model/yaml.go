package model

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// UnmarshalYAML implements custom YAML unmarshalling for StringOrList,
// accepting either a single string or a list of strings.
func (s *StringOrList) UnmarshalYAML(value *yaml.Node) error {
	switch value.Kind {
	case yaml.ScalarNode:
		*s = StringOrList{value.Value}
		return nil
	case yaml.SequenceNode:
		var list []string
		if err := value.Decode(&list); err != nil {
			return fmt.Errorf("decoding string list: %w", err)
		}
		*s = StringOrList(list)
		return nil
	default:
		return fmt.Errorf("expected string or list, got %v", value.Kind)
	}
}

// MarshalYAML writes a single string as a scalar, multiple strings as a
// compact flow-style sequence (e.g. [a, b, c]).
func (s StringOrList) MarshalYAML() (interface{}, error) {
	if len(s) == 1 {
		return s[0], nil
	}
	node := &yaml.Node{
		Kind:  yaml.SequenceNode,
		Style: yaml.FlowStyle,
	}
	for _, v := range s {
		node.Content = append(node.Content, &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: v,
		})
	}
	return node, nil
}

// MarshalYAML writes Input as a compact flow-style map so inputs stay on
// one line (e.g. `{ from: fixer, fallback: external }`) instead of expanding
// into multiple lines.
func (i Input) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{
		Kind:  yaml.MappingNode,
		Style: yaml.FlowStyle,
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "from"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: i.From},
	)
	if i.Fallback != "" {
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "fallback"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: i.Fallback},
		)
	}
	return node, nil
}

// MarshalYAML writes AlwaysCondition as a flow-style map to keep it compact.
func (a AlwaysCondition) MarshalYAML() (interface{}, error) {
	node := &yaml.Node{
		Kind:  yaml.MappingNode,
		Style: yaml.FlowStyle,
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "max_runs"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("%d", a.MaxRuns)},
	)
	return node, nil
}
