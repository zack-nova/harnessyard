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

var flatMemberHintKeys = map[string]struct{}{
	"name":        {},
	"description": {},
	"role":        {},
	"lane":        {},
	"scopes":      {},
}

type memberHintBody struct {
	Name        string                 `yaml:"name,omitempty"`
	Description string                 `yaml:"description,omitempty"`
	Role        OrbitMemberRole        `yaml:"role,omitempty"`
	Lane        string                 `yaml:"lane,omitempty"`
	Scopes      *OrbitMemberScopePatch `yaml:"scopes,omitempty"`
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

	root, foundRoot, err := yamlMappingRoot(frontmatterContent, normalizedHintPath, "frontmatter")
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
	flatHint := false
	if !found {
		orbitMemberData, found, err = extractFlatMemberHintMapping(root, normalizedHintPath)
		if err != nil {
			return resolvedMemberHint{}, false, err
		}
		if !found {
			return resolvedMemberHint{}, false, nil
		}
		flatHint = true
	}

	var body memberHintBody
	if err := contractutil.DecodeKnownFields(orbitMemberData, &body); err != nil {
		hintLabel := "orbit_member"
		if flatHint {
			hintLabel = "flat member hint"
		}
		return resolvedMemberHint{}, false, fmt.Errorf("%s %s is invalid YAML: %w", normalizedHintPath, hintLabel, err)
	}

	var hint resolvedMemberHint
	if flatHint {
		hint, err = resolveFlatMemberHint(memberHintKindFileFrontmatter, normalizedHintPath, normalizedHintPath, body)
	} else {
		hint, err = resolveMemberHint(memberHintKindFileFrontmatter, normalizedHintPath, normalizedHintPath, body)
	}
	if err != nil {
		return resolvedMemberHint{}, false, err
	}

	return hint, true, nil
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
		var body memberHintBody
		if err := contractutil.DecodeKnownFields(orbitMemberData, &body); err != nil {
			return resolvedMemberHint{}, fmt.Errorf("%s orbit_member is invalid YAML: %w", normalizedMarkerPath, err)
		}
		return resolveMemberHint(memberHintKindDirectoryMarker, normalizedMarkerPath, rootPath, body)
	}

	flatData, found, err := extractFlatMemberHintMapping(root, normalizedMarkerPath)
	if err != nil {
		return resolvedMemberHint{}, err
	}
	if !found {
		return resolvedMemberHint{}, fmt.Errorf("%s must define orbit_member or flat member hint fields", normalizedMarkerPath)
	}
	var body memberHintBody
	if err := contractutil.DecodeKnownFields(flatData, &body); err != nil {
		return resolvedMemberHint{}, fmt.Errorf("%s flat member hint is invalid YAML: %w", normalizedMarkerPath, err)
	}
	return resolveFlatMemberHint(memberHintKindDirectoryMarker, normalizedMarkerPath, rootPath, body)
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
	return resolveMemberHintWithOptions(kind, hintPath, rootPath, body, false, defaultMemberHintRole(kind), "orbit_member")
}

func resolveFlatMemberHint(kind string, hintPath string, rootPath string, body memberHintBody) (resolvedMemberHint, error) {
	return resolveMemberHintWithOptions(kind, hintPath, rootPath, body, true, OrbitMemberRule, "member_hint")
}

func resolveMemberHintWithOptions(
	kind string,
	hintPath string,
	rootPath string,
	body memberHintBody,
	requireName bool,
	defaultRole OrbitMemberRole,
	fieldPrefix string,
) (resolvedMemberHint, error) {
	name := strings.TrimSpace(body.Name)
	if name == "" {
		if requireName {
			return resolvedMemberHint{}, fmt.Errorf("%s.name is required", fieldPrefix)
		}
		name = defaultMemberHintName(kind, rootPath)
	}
	if err := validateMemberHintNameWithPrefix(name, fieldPrefix+".name"); err != nil {
		return resolvedMemberHint{}, err
	}

	role := body.Role
	if role == "" {
		role = defaultRole
	}
	if !role.IsValid() {
		return resolvedMemberHint{}, fmt.Errorf("%s.role: invalid orbit member role %q", fieldPrefix, role)
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
		Scopes:      cloneOrbitMemberScopePatch(body.Scopes),
	}, nil
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

func extractFlatMemberHintMapping(root *yaml.Node, hintPath string) ([]byte, bool, error) {
	if !isFlatMemberHintRoot(root) {
		return nil, false, nil
	}

	data, err := yaml.Marshal(root)
	if err != nil {
		return nil, false, fmt.Errorf("%s flat member hint marshal failed: %w", hintPath, err)
	}

	return data, true, nil
}

func isFlatMemberHintRoot(root *yaml.Node) bool {
	hasName := false
	for index := 0; index+1 < len(root.Content); index += 2 {
		keyNode := root.Content[index]
		if _, ok := flatMemberHintKeys[keyNode.Value]; !ok {
			return false
		}
		if keyNode.Value == "name" {
			hasName = true
		}
	}

	return hasName
}

func cloneOrbitMemberScopePatch(scopes *OrbitMemberScopePatch) *OrbitMemberScopePatch {
	if scopes == nil {
		return nil
	}

	cloned := *scopes

	return &cloned
}
