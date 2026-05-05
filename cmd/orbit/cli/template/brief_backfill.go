package orbittemplate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/internal/contractutil"
	orbitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	"gopkg.in/yaml.v3"
)

const (
	runtimeAgentsRepoPath = "AGENTS.md"
	briefBackfillVarsPath = ".harness/vars.yaml"
)

// ErrGuidanceRootArtifactMissing reports that an optional root guidance
// artifact is absent from the current worktree.
var ErrGuidanceRootArtifactMissing = errors.New("root guidance artifact is missing")

// GuidanceBackfillStatus describes how one backfill request affected hosted truth.
type GuidanceBackfillStatus string

const (
	GuidanceBackfillStatusUpdated GuidanceBackfillStatus = "updated"
	GuidanceBackfillStatusRemoved GuidanceBackfillStatus = "removed"
	GuidanceBackfillStatusSkipped GuidanceBackfillStatus = "skipped"
)

// BriefBackfillInput captures the repo and orbit targeted by one explicit brief backfill.
type BriefBackfillInput struct {
	RepoRoot string
	OrbitID  string
}

// BriefBackfillResult reports the hosted definition updated by one successful backfill.
type BriefBackfillResult struct {
	OrbitID        string
	DefinitionPath string
	Status         GuidanceBackfillStatus
	Replacements   []ReplacementSummary
}

type reverseReplacementOccurrence struct {
	Variable string
	Literal  string
	Start    int
	End      int
}

type backfillOrbitGuidanceTemplateInput struct {
	RepoRoot       string
	OrbitID        string
	RuntimePath    string
	ContainerLabel string
	MetaField      string
}

type backfillOrbitGuidanceTemplateResult struct {
	OrbitID        string
	DefinitionPath string
	Status         GuidanceBackfillStatus
	Replacements   []ReplacementSummary
}

// BackfillOrbitBrief extracts one current revision root AGENTS block, reverse-variableizes it,
// and writes the result back into meta.agents_template.
func BackfillOrbitBrief(ctx context.Context, input BriefBackfillInput) (BriefBackfillResult, error) {
	result, err := backfillOrbitGuidanceTemplate(ctx, backfillOrbitGuidanceTemplateInput{
		RepoRoot:       input.RepoRoot,
		OrbitID:        input.OrbitID,
		RuntimePath:    runtimeAgentsRepoPath,
		ContainerLabel: "root AGENTS.md",
		MetaField:      "agents_template",
	})
	if err != nil {
		return BriefBackfillResult{}, err
	}

	return BriefBackfillResult(result), nil
}

// ReverseVariableizeBrief converts runtime literals back into variable placeholders.
func ReverseVariableizeBrief(content []byte, variables map[string]bindings.VariableBinding) (ReplacementResult, error) {
	result := ReplacementResult{
		Content: append([]byte(nil), content...),
	}

	entries, ambiguities, err := buildReplacementPlan(variables)
	if err != nil {
		return ReplacementResult{}, fmt.Errorf("build reverse replacement plan: %w", err)
	}
	if len(ambiguities) > 0 {
		return ReplacementResult{}, fmt.Errorf("reverse replacement is ambiguous: %s", formatReplacementAmbiguities(ambiguities))
	}

	if left, right, ok := findOverlappingReplacementEntries(string(content), entries); ok {
		return ReplacementResult{}, fmt.Errorf(
			"reverse replacement is ambiguous: overlapping runtime values %q (%s) and %q (%s)",
			left.Literal,
			left.Variable,
			right.Literal,
			right.Variable,
		)
	}

	text := string(content)
	summaries := make([]ReplacementSummary, 0, len(entries))
	for _, entry := range entries {
		count := strings.Count(text, entry.Literal)
		if count == 0 {
			continue
		}

		text = strings.ReplaceAll(text, entry.Literal, "$"+entry.Variable)
		summaries = append(summaries, ReplacementSummary{
			Variable: entry.Variable,
			Literal:  entry.Literal,
			Count:    count,
		})
	}

	sort.Slice(summaries, func(left, right int) bool {
		if summaries[left].Count == summaries[right].Count {
			return summaries[left].Variable < summaries[right].Variable
		}
		return summaries[left].Count > summaries[right].Count
	})

	result.Content = []byte(text)
	result.Replacements = summaries

	return result, nil
}

func ensureBriefBackfillRevisionAllowed(repoRoot string) error {
	_, err := resolveAllowedBriefRevisionKind(repoRoot, "backfill")
	return err
}

func resolveAllowedBriefRevisionKind(repoRoot string, operation string) (string, error) {
	revisionKind, err := resolveBriefRevisionKind(repoRoot)
	if err != nil {
		return "", fmt.Errorf("load current revision manifest: %w", err)
	}

	if err := validateBriefRevisionKindAllowed(revisionKind, operation); err != nil {
		return "", err
	}

	return revisionKind, nil
}

func resolveBriefRevisionKind(repoRoot string) (string, error) {
	revisionKind, err := loadCurrentRevisionManifestKind(repoRoot)
	if errors.Is(err, os.ErrNotExist) {
		return "plain", nil
	}
	if err != nil {
		return "", err
	}

	return revisionKind, nil
}

func validateBriefRevisionKindAllowed(revisionKind string, operation string) error {
	switch revisionKind {
	case "", "plain":
		return fmt.Errorf(`brief %s supports only runtime, source, or orbit_template revisions; current revision kind is "plain"`, operation)
	case "runtime", "source", "orbit_template":
		return nil
	default:
		return fmt.Errorf("brief %s supports only runtime, source, or orbit_template revisions; current revision kind is %q", operation, revisionKind)
	}
}

func extractRuntimeAgentsBlock(document AgentsRuntimeDocument, orbitID string) ([]byte, error) {
	for _, segment := range document.Segments {
		if segment.Kind != AgentsRuntimeSegmentBlock {
			continue
		}
		if segment.OwnerKind != OwnerKindOrbit || segment.WorkflowID != orbitID {
			continue
		}

		return append([]byte(nil), segment.Content...), nil
	}

	return nil, fmt.Errorf("root AGENTS.md does not contain orbit block %q", orbitID)
}

func loadOptionalRuntimeVars(repoRoot string) (map[string]bindings.VariableBinding, error) {
	filename := filepath.Join(repoRoot, filepath.FromSlash(briefBackfillVarsPath))
	file, err := bindings.LoadVarsFileAtPath(filename)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]bindings.VariableBinding{}, nil
		}
		return nil, fmt.Errorf("load .harness/vars.yaml: %w", err)
	}

	return file.Variables, nil
}

func formatReplacementAmbiguities(ambiguities []ReplacementAmbiguity) string {
	parts := make([]string, 0, len(ambiguities))
	for _, ambiguity := range ambiguities {
		parts = append(parts, fmt.Sprintf("%q => %s", ambiguity.Literal, strings.Join(ambiguity.Variables, ", ")))
	}
	sort.Strings(parts)

	return strings.Join(parts, "; ")
}

func findOverlappingReplacementEntries(content string, entries []replacementEntry) (reverseReplacementOccurrence, reverseReplacementOccurrence, bool) {
	occurrences := make([]reverseReplacementOccurrence, 0)
	for _, entry := range entries {
		start := 0
		for {
			offset := strings.Index(content[start:], entry.Literal)
			if offset < 0 {
				break
			}

			matchStart := start + offset
			occurrences = append(occurrences, reverseReplacementOccurrence{
				Variable: entry.Variable,
				Literal:  entry.Literal,
				Start:    matchStart,
				End:      matchStart + len(entry.Literal),
			})
			start = matchStart + 1
		}
	}

	sort.Slice(occurrences, func(left, right int) bool {
		if occurrences[left].Start == occurrences[right].Start {
			if occurrences[left].End == occurrences[right].End {
				return occurrences[left].Variable < occurrences[right].Variable
			}
			return occurrences[left].End < occurrences[right].End
		}
		return occurrences[left].Start < occurrences[right].Start
	})

	for left := 0; left < len(occurrences); left++ {
		for right := left + 1; right < len(occurrences); right++ {
			if occurrences[right].Start >= occurrences[left].End {
				break
			}
			if occurrences[left].Literal == occurrences[right].Literal {
				continue
			}

			return occurrences[left], occurrences[right], true
		}
	}

	return reverseReplacementOccurrence{}, reverseReplacementOccurrence{}, false
}

func backfillOrbitGuidanceTemplate(ctx context.Context, input backfillOrbitGuidanceTemplateInput) (backfillOrbitGuidanceTemplateResult, error) {
	if strings.TrimSpace(input.RepoRoot) == "" {
		return backfillOrbitGuidanceTemplateResult{}, errors.New("repo root must not be empty")
	}
	if err := ensureBriefBackfillRevisionAllowed(input.RepoRoot); err != nil {
		return backfillOrbitGuidanceTemplateResult{}, err
	}

	runtimeData, err := os.ReadFile(filepath.Join(input.RepoRoot, filepath.FromSlash(input.RuntimePath)))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("%s is missing: %w", input.ContainerLabel, ErrGuidanceRootArtifactMissing)
		}
		return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("read %s: %w", input.ContainerLabel, err)
	}

	document, err := ParseRuntimeAgentsDocument(runtimeData)
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("parse %s: %w", input.ContainerLabel, err)
	}

	payload, err := extractRuntimeGuidanceBlock(document, input.OrbitID, input.ContainerLabel)
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, err
	}

	runtimeBindings, err := loadOptionalRuntimeVars(input.RepoRoot)
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, err
	}

	replaced, err := ReverseVariableizeBrief(payload, runtimeBindings)
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, err
	}

	spec, err := orbitpkg.LoadHostedOrbitSpec(ctx, input.RepoRoot, input.OrbitID)
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("load hosted orbit spec: %w", err)
	}
	if !spec.HasMemberSchema() || spec.Meta == nil {
		return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("hosted orbit %q must use member schema before guidance backfill", input.OrbitID)
	}

	definitionPath, status, err := writeHostedMetaTemplatePreservingDocument(input.RepoRoot, spec.ID, input.MetaField, string(replaced.Content))
	if err != nil {
		return backfillOrbitGuidanceTemplateResult{}, fmt.Errorf("write hosted orbit spec: %w", err)
	}

	return backfillOrbitGuidanceTemplateResult{
		OrbitID:        input.OrbitID,
		DefinitionPath: definitionPath,
		Status:         status,
		Replacements:   replaced.Replacements,
	}, nil
}

func writeHostedMetaTemplatePreservingDocument(repoRoot string, orbitID string, metaField string, template string) (string, GuidanceBackfillStatus, error) {
	filename, err := orbitpkg.HostedDefinitionPath(repoRoot, orbitID)
	if err != nil {
		return "", "", fmt.Errorf("build hosted orbit spec path: %w", err)
	}

	//nolint:gosec // The hosted definition path is validated from the orbit id under the repo root.
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", "", fmt.Errorf("read hosted orbit spec: %w", err)
	}

	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return "", "", fmt.Errorf("parse hosted orbit spec document: %w", err)
	}

	root, err := hostedOrbitSpecDocumentRoot(&document)
	if err != nil {
		return "", "", err
	}
	metaNode, err := hostedOrbitSpecMetaMapping(root)
	if err != nil {
		return "", "", err
	}
	status, err := updateHostedMetaTemplateNode(metaNode, metaField, template)
	if err != nil {
		return "", "", err
	}
	if status == GuidanceBackfillStatusSkipped {
		return filename, status, nil
	}

	encoded, err := contractutil.EncodeYAMLDocument(root)
	if err != nil {
		return "", "", fmt.Errorf("encode hosted orbit spec document: %w", err)
	}
	if bytes.Equal(data, encoded) {
		return filename, status, nil
	}
	if err := contractutil.AtomicWriteFileMode(filename, encoded, 0o600); err != nil {
		return "", "", fmt.Errorf("write hosted orbit spec document: %w", err)
	}

	return filename, status, nil
}

func hostedOrbitSpecDocumentRoot(document *yaml.Node) (*yaml.Node, error) {
	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec document shape")
	}
	root := document.Content[0]
	if root.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec document alias")
	}
	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec root mapping")
	}

	return root, nil
}

func hostedOrbitSpecMetaMapping(root *yaml.Node) (*yaml.Node, error) {
	metaNode, found := mappingValueNode(root, "meta")
	if !found {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec without meta mapping")
	}
	if metaNode.Kind == yaml.AliasNode {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec meta mapping alias")
	}
	if metaNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("cannot safely preserve hosted orbit spec meta mapping")
	}

	return metaNode, nil
}

func mappingValueNode(mapping *yaml.Node, key string) (*yaml.Node, bool) {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		keyNode := mapping.Content[index]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}
		if keyNode.Value != key {
			continue
		}

		return mapping.Content[index+1], true
	}

	return nil, false
}

func updateHostedMetaTemplateNode(metaNode *yaml.Node, fieldName string, template string) (GuidanceBackfillStatus, error) {
	if strings.TrimSpace(template) == "" {
		if removeHostedMetaTemplateNode(metaNode, fieldName) {
			return GuidanceBackfillStatusRemoved, nil
		}
		return GuidanceBackfillStatusSkipped, nil
	}
	currentValue, found := mappingValueNode(metaNode, fieldName)
	if found {
		if currentValue.Kind == yaml.AliasNode {
			return "", fmt.Errorf("cannot safely preserve existing meta.%s alias", fieldName)
		}
		if currentValue.Kind == yaml.ScalarNode && currentValue.Value == template {
			return GuidanceBackfillStatusSkipped, nil
		}
	}

	if err := setHostedMetaTemplateNode(metaNode, fieldName, template); err != nil {
		return "", err
	}

	return GuidanceBackfillStatusUpdated, nil
}

func removeHostedMetaTemplateNode(metaNode *yaml.Node, fieldName string) bool {
	for index := 0; index+1 < len(metaNode.Content); index += 2 {
		keyNode := metaNode.Content[index]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value != fieldName {
			continue
		}

		metaNode.Content = append(metaNode.Content[:index], metaNode.Content[index+2:]...)
		return true
	}

	return false
}

func setHostedMetaTemplateNode(metaNode *yaml.Node, fieldName string, template string) error {
	replacement := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: template,
		Style: yaml.LiteralStyle,
	}

	for index := 0; index+1 < len(metaNode.Content); index += 2 {
		keyNode := metaNode.Content[index]
		if keyNode.Kind != yaml.ScalarNode || keyNode.Value != fieldName {
			continue
		}

		currentValue := metaNode.Content[index+1]
		if currentValue.Kind == yaml.AliasNode {
			return fmt.Errorf("cannot safely preserve existing meta.%s alias", fieldName)
		}
		replacement.HeadComment = currentValue.HeadComment
		replacement.LineComment = currentValue.LineComment
		replacement.FootComment = currentValue.FootComment
		metaNode.Content[index+1] = replacement
		return nil
	}

	metaNode.Content = append(metaNode.Content, &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: fieldName,
	}, replacement)

	return nil
}
