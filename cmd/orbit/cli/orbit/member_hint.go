package orbit

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
)

const (
	memberHintKindFileFrontmatter = "file_frontmatter"
	memberHintKindDirectoryMarker = "directory_marker"
	memberHintMarkerFileName      = ".orbit-member.yaml"
)

type memberHintBody struct {
	Name        string          `yaml:"name,omitempty"`
	Description string          `yaml:"description,omitempty"`
	Role        OrbitMemberRole `yaml:"role,omitempty"`
	Lane        string          `yaml:"lane,omitempty"`
}

type resolvedMemberHint struct {
	Kind        string
	HintPath    string
	RootPath    string
	Name        string
	Description string
	Role        OrbitMemberRole
	Lane        string
	Scopes      *OrbitMemberScopePatch
}

type memberHintCandidate struct {
	Hint   resolvedMemberHint
	Member OrbitMember
}

func parseMarkdownMemberHint(hintPath string, data []byte) (resolvedMemberHint, bool, error) {
	normalizedHintPath, err := ids.NormalizeRepoRelativePath(hintPath)
	if err != nil {
		return resolvedMemberHint{}, false, fmt.Errorf("normalize member hint path: %w", err)
	}
	if path.Ext(normalizedHintPath) != ".md" {
		return resolvedMemberHint{}, false, fmt.Errorf("member hint file %q must be a Markdown file", normalizedHintPath)
	}

	frontmatterContent, hasFrontmatter, err := extractYAMLFrontmatter(normalizedHintPath, data)
	if err != nil {
		return resolvedMemberHint{}, false, err
	}
	if !hasFrontmatter {
		return resolvedMemberHint{}, false, nil
	}

	root, foundRoot, err := markdownFrontmatterMappingRoot(frontmatterContent, normalizedHintPath)
	if err != nil {
		return resolvedMemberHint{}, false, err
	}
	if !foundRoot {
		return resolvedMemberHint{}, false, nil
	}

	orbitMemberData, found, err := extractOrbitMemberMapping(root, normalizedHintPath)
	if err != nil {
		return resolvedMemberHint{}, false, err
	}
	if !found {
		return resolvedMemberHint{}, false, nil
	}

	var body memberHintBody
	if err := contractutil.DecodeKnownFields(orbitMemberData, &body); err != nil {
		return resolvedMemberHint{}, false, fmt.Errorf("%s orbit_member is invalid YAML: %w", normalizedHintPath, err)
	}

	hint, err := resolveMemberHint(memberHintKindFileFrontmatter, normalizedHintPath, normalizedHintPath, body)
	if err != nil {
		return resolvedMemberHint{}, false, err
	}

	return hint, true, nil
}

func markdownFrontmatterMappingRoot(data []byte, hintPath string) (*yaml.Node, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		if markdownFrontmatterDeclaresOrbitMember(data) {
			return nil, false, fmt.Errorf("%s frontmatter is invalid YAML: %w", hintPath, err)
		}
		return nil, false, nil
	}
	if len(document.Content) == 0 {
		return nil, false, nil
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, nil
	}

	return root, true, nil
}

func markdownFrontmatterDeclaresOrbitMember(data []byte) bool {
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		if line[0] == ' ' || line[0] == '\t' {
			continue
		}

		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		key, _, found := strings.Cut(trimmed, ":")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		key = strings.Trim(key, `"'`)
		if key == "orbit_member" {
			return true
		}
	}

	return false
}

func parseDirectoryMemberHint(markerPath string, data []byte) (resolvedMemberHint, error) {
	normalizedMarkerPath, err := ids.NormalizeRepoRelativePath(markerPath)
	if err != nil {
		return resolvedMemberHint{}, fmt.Errorf("normalize member marker path: %w", err)
	}
	if path.Base(normalizedMarkerPath) != memberHintMarkerFileName {
		return resolvedMemberHint{}, fmt.Errorf("member marker path %q must end with %s", normalizedMarkerPath, memberHintMarkerFileName)
	}

	root, foundRoot, err := yamlMappingRoot(data, normalizedMarkerPath, "")
	if err != nil {
		return resolvedMemberHint{}, err
	}
	if !foundRoot {
		return resolvedMemberHint{}, fmt.Errorf("%s must define member hint fields", normalizedMarkerPath)
	}

	rootPath := path.Dir(normalizedMarkerPath)
	if rootPath == "." {
		return resolvedMemberHint{}, fmt.Errorf("%s must live inside a directory", normalizedMarkerPath)
	}

	orbitMemberData, found, err := extractOrbitMemberMapping(root, normalizedMarkerPath)
	if err != nil {
		return resolvedMemberHint{}, err
	}
	if found {
		if err := rejectDirectoryMarkerTopLevelFields(root, normalizedMarkerPath); err != nil {
			return resolvedMemberHint{}, err
		}
		var body memberHintBody
		if err := contractutil.DecodeKnownFields(orbitMemberData, &body); err != nil {
			return resolvedMemberHint{}, fmt.Errorf("%s orbit_member is invalid YAML: %w", normalizedMarkerPath, err)
		}
		return resolveMemberHint(memberHintKindDirectoryMarker, normalizedMarkerPath, rootPath, body)
	}

	return resolvedMemberHint{}, fmt.Errorf("%s must define nested orbit_member", normalizedMarkerPath)
}

func rejectDirectoryMarkerTopLevelFields(root *yaml.Node, markerPath string) error {
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		if keyNode.Value != "orbit_member" {
			return fmt.Errorf("%s top-level field %q is not supported; use nested orbit_member", markerPath, keyNode.Value)
		}
	}

	return nil
}

func buildMemberHintCandidate(hint resolvedMemberHint) memberHintCandidate {
	includePath := hint.RootPath
	if hint.Kind == memberHintKindDirectoryMarker {
		includePath = hint.RootPath + "/**"
	}

	return memberHintCandidate{
		Hint: hint,
		Member: OrbitMember{
			Name:        hint.Name,
			Description: hint.Description,
			Role:        hint.Role,
			Paths: OrbitMemberPaths{
				Include: []string{includePath},
			},
			Lane:   hint.Lane,
			Scopes: cloneOrbitMemberScopePatch(hint.Scopes),
		},
	}
}

func isHintManageableMember(member OrbitMember) bool {
	return len(member.Paths.Include) == 1 && len(member.Paths.Exclude) == 0
}

func resolveMemberHint(kind string, hintPath string, rootPath string, body memberHintBody) (resolvedMemberHint, error) {
	fieldPrefix := "orbit_member"
	name := strings.TrimSpace(body.Name)
	if name == "" {
		name = defaultMemberHintName(kind, rootPath)
	}
	if err := validateMemberHintNameWithPrefix(name, fieldPrefix+".name"); err != nil {
		return resolvedMemberHint{}, err
	}

	role := body.Role
	if role == "" {
		role = defaultMemberHintRole(kind)
	}
	if !role.IsValid() {
		return resolvedMemberHint{}, fmt.Errorf("%s.role: invalid orbit member role %q", fieldPrefix, role)
	}
	if !isValidMemberHintRole(role) {
		return resolvedMemberHint{}, fmt.Errorf("%s.role: invalid member hint role %q", fieldPrefix, role)
	}

	lane := strings.TrimSpace(body.Lane)
	if lane != "" && lane != OrbitMemberLaneBootstrap {
		return resolvedMemberHint{}, fmt.Errorf(`%s.lane must be %q when present`, fieldPrefix, OrbitMemberLaneBootstrap)
	}

	return resolvedMemberHint{
		Kind:        kind,
		HintPath:    hintPath,
		RootPath:    rootPath,
		Name:        name,
		Description: strings.TrimSpace(body.Description),
		Role:        role,
		Lane:        lane,
	}, nil
}

func isValidMemberHintRole(role OrbitMemberRole) bool {
	switch role {
	case OrbitMemberSubject, OrbitMemberRule, OrbitMemberProcess:
		return true
	default:
		return false
	}
}

func defaultMemberHintName(kind string, rootPath string) string {
	base := path.Base(rootPath)
	if kind == memberHintKindFileFrontmatter {
		return strings.TrimSuffix(base, path.Ext(base))
	}

	return base
}

func defaultMemberHintRole(kind string) OrbitMemberRole {
	if kind == memberHintKindDirectoryMarker {
		return OrbitMemberProcess
	}

	return OrbitMemberRule
}

func validateMemberHintNameWithPrefix(name string, fieldName string) error {
	if err := ids.ValidateOrbitID(name); err != nil {
		return fmt.Errorf("%s: %w", fieldName, err)
	}
	if name == orbitSpecMemberName {
		return fmt.Errorf(`%s %q is reserved`, fieldName, name)
	}

	return nil
}

func extractYAMLFrontmatter(hintPath string, data []byte) ([]byte, bool, error) {
	content := strings.ReplaceAll(string(data), "\r\n", "\n")
	if !strings.HasPrefix(content, "---\n") {
		return nil, false, nil
	}

	rest := strings.TrimPrefix(content, "---\n")
	end := strings.Index(rest, "\n---\n")
	if end < 0 {
		return nil, false, fmt.Errorf("%s frontmatter must terminate with ---", hintPath)
	}

	return []byte(rest[:end]), true, nil
}

func yamlMappingRoot(data []byte, hintPath string, context string) (*yaml.Node, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		if context != "" {
			return nil, false, fmt.Errorf("%s %s is invalid YAML: %w", hintPath, context, err)
		}
		return nil, false, fmt.Errorf("%s is invalid YAML: %w", hintPath, err)
	}
	if len(document.Content) == 0 {
		return nil, false, nil
	}

	root := document.Content[0]
	if root.Kind != yaml.MappingNode {
		if context != "" {
			return nil, false, fmt.Errorf("%s %s must be a mapping", hintPath, context)
		}
		return nil, false, fmt.Errorf("%s must be a mapping", hintPath)
	}

	return root, true, nil
}

func extractOrbitMemberMapping(root *yaml.Node, hintPath string) ([]byte, bool, error) {
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		valueNode := root.Content[index+1]
		if keyNode.Value != "orbit_member" {
			continue
		}
		if valueNode.Kind != yaml.MappingNode {
			return nil, false, fmt.Errorf("%s orbit_member must be a mapping", hintPath)
		}

		data, err := yaml.Marshal(valueNode)
		if err != nil {
			return nil, false, fmt.Errorf("%s orbit_member marshal failed: %w", hintPath, err)
		}

		return data, true, nil
	}

	return nil, false, nil
}

func cloneOrbitMemberScopePatch(scopes *OrbitMemberScopePatch) *OrbitMemberScopePatch {
	if scopes == nil {
		return nil
	}

	cloned := *scopes

	return &cloned
}
