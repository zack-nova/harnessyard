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
	varsRelativePath          = ".harness/vars.yaml"
	VarsSchemaVersion         = 2
	varsSchemaVersion         = VarsSchemaVersion
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
	Value       string       `yaml:"value,omitempty"`
	ValueFrom   *ValueSource `yaml:"value_from,omitempty"`
	Description string       `yaml:"description,omitempty"`
}

// ValueSource stores one explicit source for a Runtime Binding value.
type ValueSource struct {
	Env  string `yaml:"env,omitempty"`
	File string `yaml:"file,omitempty"`
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
	Value       *string         `yaml:"value"`
	ValueFrom   *rawValueSource `yaml:"value_from"`
	Description *string         `yaml:"description"`
	NotMapping  bool            `yaml:"-"`
}

type rawValueSource struct {
	Env        *string `yaml:"env"`
	File       *string `yaml:"file"`
	NotMapping bool    `yaml:"-"`
}

// UnmarshalYAML records scalar shorthand so validation can report the binding path.
func (raw *rawVariableBinding) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		raw.NotMapping = true
		return nil
	case yaml.MappingNode:
		var value *string
		var valueFrom *rawValueSource
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
			case "value_from":
				var decoded rawValueSource
				if err := valueNode.Decode(&decoded); err != nil {
					return fmt.Errorf("decode value_from: %w", err)
				}
				valueFrom = &decoded
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
		raw.ValueFrom = valueFrom
		raw.Description = description
		raw.NotMapping = false
		return nil
	default:
		return fmt.Errorf("cannot unmarshal %s into bindings.rawVariableBinding", node.ShortTag())
	}
}

// UnmarshalYAML records invalid source shapes so validation can report the binding path.
func (raw *rawValueSource) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		raw.NotMapping = true
		return nil
	case yaml.MappingNode:
		var env *string
		var file *string
		for index := 0; index < len(node.Content); index += 2 {
			key := node.Content[index].Value
			valueNode := node.Content[index+1]

			switch key {
			case "env":
				var decoded string
				if err := valueNode.Decode(&decoded); err != nil {
					return fmt.Errorf("decode env: %w", err)
				}
				env = &decoded
			case "file":
				var decoded string
				if err := valueNode.Decode(&decoded); err != nil {
					return fmt.Errorf("decode file: %w", err)
				}
				file = &decoded
			default:
				return fmt.Errorf("field %q not found in type bindings.rawValueSource", key)
			}
		}

		raw.Env = env
		raw.File = file
		raw.NotMapping = false
		return nil
	default:
		return fmt.Errorf("cannot unmarshal %s into bindings.rawValueSource", node.ShortTag())
	}
}

// VarsPath returns the absolute path to .harness/vars.yaml.
func VarsPath(repoRoot string) string {
	return filepath.Join(repoRoot, filepath.FromSlash(varsRelativePath))
}

// LoadVarsFile reads, decodes, and validates the bindings file at the canonical Runtime Bindings path.
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

// LoadVarsFileWorktreeOrHEAD reads the bindings file from the canonical Runtime Bindings path.
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

// ParseVarsData decodes and validates Runtime Bindings bytes.
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
		if err := validateVariableBinding(fmt.Sprintf("variables.%s", name), file.Variables[name]); err != nil {
			return err
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
			if err := validateVariableBinding(
				fmt.Sprintf("%s.%s.%s.%s", scopedVariablesFieldName, namespace, scopedVariablesNestedName, name),
				scoped.Variables[name],
			); err != nil {
				return err
			}
		}
	}

	return nil
}

// WriteVarsFile validates and writes the bindings file at the canonical Runtime Bindings path.
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
	if raw.NotMapping {
		return VariableBinding{}, fmt.Errorf("%s must be a mapping with value or value_from", prefix)
	}
	if (raw.Value == nil) == (raw.ValueFrom == nil) {
		return VariableBinding{}, fmt.Errorf("%s must set exactly one of value or value_from", prefix)
	}

	binding := VariableBinding{}
	if raw.Value != nil {
		binding.Value = *raw.Value
	} else {
		valueFrom, err := raw.ValueFrom.toValueSource(prefix + ".value_from")
		if err != nil {
			return VariableBinding{}, err
		}
		binding.ValueFrom = &valueFrom
	}
	if raw.Description != nil {
		binding.Description = *raw.Description
	}

	return binding, nil
}

func (raw rawValueSource) toValueSource(prefix string) (ValueSource, error) {
	if raw.NotMapping {
		return ValueSource{}, fmt.Errorf("%s must be a mapping with env or file", prefix)
	}

	hasEnv := raw.Env != nil
	hasFile := raw.File != nil
	if hasEnv == hasFile {
		return ValueSource{}, fmt.Errorf("%s must set exactly one of env or file", prefix)
	}
	if hasEnv {
		if strings.TrimSpace(*raw.Env) == "" {
			return ValueSource{}, fmt.Errorf("%s.env must not be blank", prefix)
		}
		return ValueSource{Env: *raw.Env}, nil
	}
	if strings.TrimSpace(*raw.File) == "" {
		return ValueSource{}, fmt.Errorf("%s.file must not be blank", prefix)
	}
	return ValueSource{File: *raw.File}, nil
}

func validateVariableBinding(prefix string, binding VariableBinding) error {
	if binding.ValueFrom == nil {
		return nil
	}
	if binding.Value != "" {
		return fmt.Errorf("%s must set exactly one of value or value_from", prefix)
	}

	return validateValueSource(prefix+".value_from", *binding.ValueFrom)
}

func validateValueSource(prefix string, source ValueSource) error {
	hasEnv := source.Env != ""
	hasFile := source.File != ""
	if hasEnv == hasFile {
		return fmt.Errorf("%s must set exactly one of env or file", prefix)
	}
	if hasEnv && strings.TrimSpace(source.Env) == "" {
		return fmt.Errorf("%s.env must not be blank", prefix)
	}
	if hasFile && strings.TrimSpace(source.File) == "" {
		return fmt.Errorf("%s.file must not be blank", prefix)
	}

	return nil
}

func varsFileNode(file VarsFile) *yaml.Node {
	root := contractutil.MappingNode()
	contractutil.AppendMapping(root, "schema_version", contractutil.IntNode(file.SchemaVersion))

	variables := contractutil.MappingNode()
	for _, name := range contractutil.SortedKeys(file.Variables) {
		binding := file.Variables[name]
		contractutil.AppendMapping(variables, name, variableBindingNode(binding))
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
				contractutil.AppendMapping(variablesNode, name, variableBindingNode(binding))
			}
			contractutil.AppendMapping(namespaceNode, scopedVariablesNestedName, variablesNode)
			contractutil.AppendMapping(scopedVariables, namespace, namespaceNode)
		}
		contractutil.AppendMapping(root, scopedVariablesFieldName, scopedVariables)
	}

	return root
}

func variableBindingNode(binding VariableBinding) *yaml.Node {
	bindingNode := contractutil.MappingNode()
	if binding.ValueFrom != nil {
		contractutil.AppendMapping(bindingNode, "value_from", valueSourceNode(*binding.ValueFrom))
	} else {
		contractutil.AppendMapping(bindingNode, "value", contractutil.StringNode(binding.Value))
	}
	if binding.Description != "" {
		contractutil.AppendMapping(bindingNode, "description", contractutil.StringNode(binding.Description))
	}

	return bindingNode
}

func valueSourceNode(source ValueSource) *yaml.Node {
	sourceNode := contractutil.MappingNode()
	if source.Env != "" {
		contractutil.AppendMapping(sourceNode, "env", contractutil.StringNode(source.Env))
	}
	if source.File != "" {
		contractutil.AppendMapping(sourceNode, "file", contractutil.StringNode(source.File))
	}

	return sourceNode
}
