package bindings

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	varsRelativePath          = ".orbit/vars.yaml"
	varsSchemaVersion         = 1
	variablesFieldName        = "variables"
	scopedVariablesFieldName  = "scoped_variables"
	scopedVariablesNestedName = "variables"
)

// VarsFile is the schema-backed versioned bindings document.
type VarsFile struct {
	SchemaVersion   int                               `yaml:"schema_version"`
	Variables       map[string]VariableBinding        `yaml:"variables"`
	ScopedVariables map[string]ScopedVariableBindings `yaml:"scoped_variables,omitempty"`
}

// ScopedVariableBindings stores bindings for one orbit-id namespace.
type ScopedVariableBindings struct {
	Variables map[string]VariableBinding `yaml:"variables"`
}

// VariableBinding stores a concrete string value and optional description.
type VariableBinding struct {
	Value       string `yaml:"value"`
	Description string `yaml:"description,omitempty"`
}

// ScopedVariablesForNamespace returns the variable map for one namespace, if present.
func ScopedVariablesForNamespace(file VarsFile, namespace string) map[string]VariableBinding {
	if file.ScopedVariables == nil {
		return nil
	}
	scoped, ok := file.ScopedVariables[namespace]
	if !ok {
		return nil
	}
	return scoped.Variables
}

type rawVarsFile struct {
	SchemaVersion   *int                                 `yaml:"schema_version"`
	Variables       map[string]rawVariableBinding        `yaml:"variables"`
	ScopedVariables map[string]rawScopedVariableBindings `yaml:"scoped_variables"`
}

type rawScopedVariableBindings struct {
	Variables map[string]rawVariableBinding `yaml:"variables"`
}

type rawVariableBinding struct {
	Value       *string `yaml:"value"`
	Description *string `yaml:"description"`
}

// UnmarshalYAML accepts both the canonical mapping form and a scalar shorthand.
func (raw *rawVariableBinding) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		if node.Tag == "!!null" {
			return nil
		}

		value := node.Value
		raw.Value = &value
		raw.Description = nil
		return nil
	case yaml.MappingNode:
		var value *string
		var description *string
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			valueNode := node.Content[index+1]

			switch key {
			case "value":
				var decoded string
				if err := valueNode.Decode(&decoded); err != nil {
					return fmt.Errorf("decode value: %w", err)
				}
				value = &decoded
			case "description":
				var decoded string
				if err := valueNode.Decode(&decoded); err != nil {
					return fmt.Errorf("decode description: %w", err)
				}
				description = &decoded
			default:
				return fmt.Errorf("field %q not found in type bindings.rawVariableBinding", key)
			}
		}

		raw.Value = value
		raw.Description = description
		return nil
	default:
		return fmt.Errorf("cannot unmarshal %s into bindings.rawVariableBinding", node.ShortTag())
	}
}

// VarsPath returns the absolute path to .orbit/vars.yaml.
func VarsPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(varsRelativePath))
}

// LoadVarsFile reads, decodes, and validates the bindings file at the fixed Phase 2 host path.
func LoadVarsFile(repoRoot string) (VarsFile, error) {
	return LoadVarsFileAtPath(VarsPath(repoRoot))
}

// LoadVarsFileAtPath reads, decodes, and validates one bindings document from an absolute path.
func LoadVarsFileAtPath(filename string) (VarsFile, error) {
	//nolint:gosec // The path is repo-local and built from the fixed bindings contract path.
	data, err := os.ReadFile(filename)
	if err != nil {
		return VarsFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseVarsData(data)
	if err != nil {
		return VarsFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// LoadVarsFileWorktreeOrHEAD reads the bindings file from the fixed Phase 2 host path.
func LoadVarsFileWorktreeOrHEAD(ctx context.Context, repoRoot string) (VarsFile, error) {
	return LoadVarsFileWorktreeOrHEADAtRepoPath(ctx, repoRoot, varsRelativePath)
}

// LoadVarsFileWorktreeOrHEADAtRepoPath reads a bindings file from the worktree when visible
// and falls back to HEAD when sparse-checkout currently hides it.
func LoadVarsFileWorktreeOrHEADAtRepoPath(ctx context.Context, repoRoot string, repoPath string) (VarsFile, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(repoPath))
	data, err := gitpkg.ReadFileWorktreeOrHEAD(ctx, repoRoot, repoPath)
	if err != nil {
		return VarsFile{}, fmt.Errorf("read %s: %w", filename, err)
	}

	file, err := ParseVarsData(data)
	if err != nil {
		return VarsFile{}, fmt.Errorf("validate %s: %w", filename, err)
	}

	return file, nil
}

// ParseVarsData decodes and validates .orbit/vars.yaml bytes.
func ParseVarsData(data []byte) (VarsFile, error) {
	var raw rawVarsFile
	if err := contractutil.DecodeKnownFields(data, &raw); err != nil {
		return VarsFile{}, fmt.Errorf("decode vars file: %w", explainVarsDecodeError(data, err))
	}

	file, err := raw.toVarsFile()
	if err != nil {
		return VarsFile{}, err
	}

	return file, nil
}

func explainVarsDecodeError(data []byte, err error) error {
	text := string(data)
	if strings.Contains(text, "${{") && strings.Contains(err.Error(), "did not find expected ',' or '}'") {
		return fmt.Errorf("%w (GitHub Actions expressions must be quoted in YAML; for example, write github_token:\n    value: '${{ secrets.GITHUB_TOKEN }}')", err)
	}

	return err
}

// ValidateVarsFile validates the bindings schema contract.
func ValidateVarsFile(file VarsFile) error {
	if file.SchemaVersion != varsSchemaVersion {
		return fmt.Errorf("schema_version must be %d", varsSchemaVersion)
	}
	if file.Variables == nil {
		return fmt.Errorf("%s must be present", variablesFieldName)
	}

	for _, name := range contractutil.SortedKeys(file.Variables) {
		if err := contractutil.ValidateVariableName(name); err != nil {
			return fmt.Errorf("variables.%s: %w", name, err)
		}
	}
	for _, namespace := range contractutil.SortedKeys(file.ScopedVariables) {
		if err := ids.ValidateOrbitID(namespace); err != nil {
			return fmt.Errorf("%s.%s: %w", scopedVariablesFieldName, namespace, err)
		}
		scoped := file.ScopedVariables[namespace]
		if scoped.Variables == nil {
			return fmt.Errorf("%s.%s.%s must be present", scopedVariablesFieldName, namespace, scopedVariablesNestedName)
		}
		for _, name := range contractutil.SortedKeys(scoped.Variables) {
			if err := contractutil.ValidateVariableName(name); err != nil {
				return fmt.Errorf("%s.%s.%s.%s: %w", scopedVariablesFieldName, namespace, scopedVariablesNestedName, name, err)
			}
		}
	}

	return nil
}

// WriteVarsFile validates and writes the bindings file at the fixed Phase 2 host path.
func WriteVarsFile(repoRoot string, file VarsFile) (string, error) {
	return WriteVarsFileAtPath(VarsPath(repoRoot), file)
}

// WriteVarsFileAtPath validates and writes one bindings document with stable field ordering.
func WriteVarsFileAtPath(filename string, file VarsFile) (string, error) {
	data, err := MarshalVarsFile(file)
	if err != nil {
		return "", fmt.Errorf("marshal %s: %w", filename, err)
	}

	if err := contractutil.AtomicWriteFile(filename, data); err != nil {
		return "", fmt.Errorf("write %s: %w", filename, err)
	}

	return filename, nil
}

// MarshalVarsFile validates and encodes a bindings document with stable field ordering.
func MarshalVarsFile(file VarsFile) ([]byte, error) {
	if err := ValidateVarsFile(file); err != nil {
		return nil, fmt.Errorf("validate vars file: %w", err)
	}

	data, err := contractutil.EncodeYAMLDocument(varsFileNode(file))
	if err != nil {
		return nil, fmt.Errorf("encode vars file: %w", err)
	}

	return data, nil
}

func (raw rawVarsFile) toVarsFile() (VarsFile, error) {
	if raw.SchemaVersion == nil {
		return VarsFile{}, fmt.Errorf("schema_version must be present")
	}
	if raw.Variables == nil {
		return VarsFile{}, fmt.Errorf("%s must be present", variablesFieldName)
	}

	file := VarsFile{
		SchemaVersion: *raw.SchemaVersion,
		Variables:     make(map[string]VariableBinding, len(raw.Variables)),
	}

	for name, rawBinding := range raw.Variables {
		binding, err := rawBinding.toVariableBinding(fmt.Sprintf("variables.%s", name))
		if err != nil {
			return VarsFile{}, err
		}
		file.Variables[name] = binding
	}
	if raw.ScopedVariables != nil {
		file.ScopedVariables = make(map[string]ScopedVariableBindings, len(raw.ScopedVariables))
		for namespace, rawScoped := range raw.ScopedVariables {
			if rawScoped.Variables == nil {
				return VarsFile{}, fmt.Errorf("%s.%s.%s must be present", scopedVariablesFieldName, namespace, scopedVariablesNestedName)
			}
			scoped := ScopedVariableBindings{
				Variables: make(map[string]VariableBinding, len(rawScoped.Variables)),
			}
			for name, rawBinding := range rawScoped.Variables {
				binding, err := rawBinding.toVariableBinding(
					fmt.Sprintf("%s.%s.%s.%s", scopedVariablesFieldName, namespace, scopedVariablesNestedName, name),
				)
				if err != nil {
					return VarsFile{}, err
				}
				scoped.Variables[name] = binding
			}
			file.ScopedVariables[namespace] = scoped
		}
	}

	if err := ValidateVarsFile(file); err != nil {
		return VarsFile{}, err
	}

	return file, nil
}

func (raw rawVariableBinding) toVariableBinding(prefix string) (VariableBinding, error) {
	if raw.Value == nil {
		return VariableBinding{}, fmt.Errorf("%s.value must be present", prefix)
	}

	binding := VariableBinding{
		Value: *raw.Value,
	}
	if raw.Description != nil {
		binding.Description = *raw.Description
	}

	return binding, nil
}

func varsFileNode(file VarsFile) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(file.SchemaVersion))

	variables := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(file.Variables) {
		binding := file.Variables[name]
		bindingNode := contractutil.MappingNode()
		contractutil.AppendMapping(bindingNode, "value", contractutil.StringNode(binding.Value))
		if binding.Description != "" {
			contractutil.AppendMapping(bindingNode, "description", contractutil.StringNode(binding.Description))
		}
		contractutil.AppendMapping(variables, name, bindingNode)
	}

	contractutil.AppendMapping(root, variablesFieldName, variables)
	if len(file.ScopedVariables) > 0 {
		scopedVariables := contractutil.MappingNode()
		for _, namespace := range contractutil.SortedKeys(file.ScopedVariables) {
			scoped := file.ScopedVariables[namespace]
			namespaceNode := contractutil.MappingNode()
			variablesNode := contractutil.MappingNode()
			for _, name := range contractutil.SortedKeys(scoped.Variables) {
				binding := scoped.Variables[name]
				bindingNode := contractutil.MappingNode()
				contractutil.AppendMapping(bindingNode, "value", contractutil.StringNode(binding.Value))
				if binding.Description != "" {
					contractutil.AppendMapping(bindingNode, "description", contractutil.StringNode(binding.Description))
				}
				contractutil.AppendMapping(variablesNode, name, bindingNode)
			}
			contractutil.AppendMapping(namespaceNode, scopedVariablesNestedName, variablesNode)
			contractutil.AppendMapping(scopedVariables, namespace, namespaceNode)
		}
		contractutil.AppendMapping(root, scopedVariablesFieldName, scopedVariables)
	}

	return root
}
