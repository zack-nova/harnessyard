package harness

import (
	"context"
	"fmt"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/orbit"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func editHarnessTemplateFiles(
	ctx context.Context,
	memberIDs []string,
	files []orbittemplate.CandidateFile,
	editor orbittemplate.Editor,
) ([]orbittemplate.CandidateFile, error) {
	editedFiles, err := orbittemplate.EditCandidateFiles(ctx, files, editor)
	if err != nil {
		return nil, fmt.Errorf("edit harness template candidate dir: %w", err)
	}

	if err := validateEditedHarnessTemplateFiles(memberIDs, editedFiles); err != nil {
		return nil, err
	}

	return editedFiles, nil
}

func validateEditedHarnessTemplateFiles(memberIDs []string, files []orbittemplate.CandidateFile) error {
	expectedDefinitionPaths := make(map[string]struct{}, len(memberIDs))
	for _, memberID := range memberIDs {
		path, err := orbit.HostedDefinitionRelativePath(memberID)
		if err != nil {
			return fmt.Errorf("build hosted member definition path for %q: %w", memberID, err)
		}
		expectedDefinitionPaths[path] = struct{}{}
	}

	presentDefinitionPaths := make(map[string]struct{}, len(expectedDefinitionPaths))
	for _, file := range files {
		switch {
		case file.Path == TemplateRepoPath():
			return fmt.Errorf("edited harness template must not write %s directly", TemplateRepoPath())
		case file.Path == ManifestRepoPath():
			return fmt.Errorf("edited harness template must not write %s directly", ManifestRepoPath())
		case strings.HasPrefix(file.Path, ".orbit/"):
			return fmt.Errorf("edited harness template must not add forbidden path %s", file.Path)
		case strings.HasPrefix(file.Path, ".harness/"):
			if _, ok := expectedDefinitionPaths[file.Path]; !ok {
				return fmt.Errorf("edited harness template must not add forbidden path %s", file.Path)
			}

			if _, err := orbit.ParseHostedOrbitSpecData(file.Content, file.Path); err != nil {
				return fmt.Errorf("validate edited member definition %s: %w", file.Path, err)
			}
			presentDefinitionPaths[file.Path] = struct{}{}
		}
	}

	for path := range expectedDefinitionPaths {
		if _, ok := presentDefinitionPaths[path]; !ok {
			return fmt.Errorf("edited harness template must keep member definition %s", path)
		}
	}

	return nil
}

func buildTemplateSaveVariableSpecs(
	memberIDs []string,
	files []orbittemplate.CandidateFile,
	runtimeBindings map[string]bindings.VariableBinding,
) (map[string]TemplateVariableSpec, error) {
	definitionPaths := make(map[string]struct{}, len(memberIDs)*2)
	for _, memberID := range memberIDs {
		hostedPath, err := orbit.HostedDefinitionRelativePath(memberID)
		if err != nil {
			return nil, fmt.Errorf("build hosted member definition path for %q: %w", memberID, err)
		}
		definitionPaths[hostedPath] = struct{}{}

		legacyPath, err := orbit.DefinitionRelativePath(memberID)
		if err != nil {
			return nil, fmt.Errorf("build legacy member definition path for %q: %w", memberID, err)
		}
		definitionPaths[legacyPath] = struct{}{}
	}

	scanFiles := make([]orbittemplate.CandidateFile, 0, len(files))
	for _, file := range files {
		if _, ok := definitionPaths[file.Path]; ok {
			continue
		}
		scanFiles = append(scanFiles, file)
	}

	scanResult := orbittemplate.ScanVariables(scanFiles, nil)
	variables := make(map[string]TemplateVariableSpec, len(scanResult.Referenced))
	for _, name := range scanResult.Referenced {
		variables[name] = TemplateVariableSpec{
			Description: runtimeBindings[name].Description,
			Required:    true,
		}
	}

	return variables, nil
}

func rootGuidanceFromTemplateFiles(files []orbittemplate.CandidateFile) RootGuidanceMetadata {
	rootGuidance := RootGuidanceMetadata{}
	for _, file := range files {
		switch file.Path {
		case rootAgentsPath:
			rootGuidance.Agents = true
		case rootHumansPath:
			rootGuidance.Humans = true
		case rootBootstrapPath:
			rootGuidance.Bootstrap = true
		}
	}

	return rootGuidance
}
