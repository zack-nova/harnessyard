package orbittemplate

import (
	"context"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/bindings"
)

// BindingsInitPreview is the non-mutating preview contract for bindings skeleton generation.
type BindingsInitPreview struct {
	Source   Source
	Manifest Manifest
	Skeleton bindings.VarsFile
}

// LocalBindingsInitInput describes bindings skeleton generation from a local template branch.
type LocalBindingsInitInput struct {
	RepoRoot  string
	SourceRef string
}

// RemoteBindingsInitInput describes bindings skeleton generation from a remote template source.
type RemoteBindingsInitInput struct {
	RepoRoot     string
	RemoteURL    string
	RequestedRef string
}

// BuildLocalBindingsInitPreview resolves a local template source and produces a fillable bindings skeleton.
func BuildLocalBindingsInitPreview(ctx context.Context, input LocalBindingsInitInput) (BindingsInitPreview, error) {
	source, err := ResolveLocalTemplateSource(ctx, input.RepoRoot, input.SourceRef)
	if err != nil {
		return BindingsInitPreview{}, err
	}

	return buildBindingsInitPreview(source, Source{
		SourceKind:     InstallSourceKindLocalBranch,
		SourceRepo:     "",
		SourceRef:      source.Ref,
		TemplateCommit: source.Commit,
	}), nil
}

// BuildRemoteBindingsInitPreview resolves a remote template source and produces a fillable bindings skeleton.
func BuildRemoteBindingsInitPreview(ctx context.Context, input RemoteBindingsInitInput) (BindingsInitPreview, error) {
	candidate, source, err := resolveRemoteTemplateSourceSnapshot(ctx, input.RepoRoot, input.RemoteURL, input.RequestedRef)
	if err != nil {
		return BindingsInitPreview{}, err
	}

	return buildBindingsInitPreview(source, Source{
		SourceKind:     InstallSourceKindExternalGit,
		SourceRepo:     candidate.RepoURL,
		SourceRef:      candidate.Branch,
		TemplateCommit: source.Commit,
	}), nil
}

func buildBindingsInitPreview(source LocalTemplateSource, templateSource Source) BindingsInitPreview {
	declared := make(map[string]bindings.VariableDeclaration, len(source.Manifest.Variables))
	for name, spec := range source.Manifest.Variables {
		declared[name] = bindings.VariableDeclaration{
			Description: spec.Description,
			Required:    spec.Required,
		}
	}

	return BindingsInitPreview{
		Source:   templateSource,
		Manifest: source.Manifest,
		Skeleton: bindings.SkeletonFromDeclarations(declared),
	}
}
