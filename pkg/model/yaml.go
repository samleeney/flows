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

// MarshalYAML writes a single string as a scalar, multiple as a sequence.
func (s StringOrList) MarshalYAML() (interface{}, error) {
	if len(s) == 1 {
		return s[0], nil
	}
	return []string(s), nil
}
