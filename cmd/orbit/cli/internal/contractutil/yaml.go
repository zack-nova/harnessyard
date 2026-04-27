package contractutil

import (
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// MappingNode returns an empty YAML mapping node.
func MappingNode() *yaml.Node {
	return &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
}

// AppendMapping appends a key/value pair to a YAML mapping node.
func AppendMapping(node *yaml.Node, key string, value *yaml.Node) {
	node.Content = append(node.Content, StringNode(key), value)
}

// StringNode returns a YAML string scalar node.
func StringNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}

// BoolNode returns a YAML bool scalar node.
func BoolNode(value bool) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!bool",
		Value: strconv.FormatBool(value),
	}
}

// IntNode returns a YAML int scalar node.
func IntNode(value int) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!int",
		Value: strconv.Itoa(value),
	}
}

// TimestampNode returns a YAML timestamp scalar node.
func TimestampNode(value time.Time) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!timestamp",
		Value: value.UTC().Format(time.RFC3339),
	}
}
