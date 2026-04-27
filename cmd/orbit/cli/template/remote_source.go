package orbittemplate

import (
	"context"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
)

// RemoteTemplateCandidate is one valid remote template branch candidate.
type RemoteTemplateCandidate struct {
	RepoURL        string
	Branch         string
	Ref            string
	Commit         string
	RequestedRef   string
	ResolutionKind RemoteTemplateResolutionKind
	Manifest       Manifest
}

type RemoteTemplateResolutionKind string

const (
	RemoteTemplateResolutionTemplateBranch RemoteTemplateResolutionKind = "template_branch"
	RemoteTemplateResolutionSourceAlias    RemoteTemplateResolutionKind = "source_alias"
)

type RemoteTemplateNotFoundReason string

const (
	RemoteTemplateNotFoundReasonSourceAliasMissingPublishOrbitID    RemoteTemplateNotFoundReason = "source_alias_missing_publish_orbit_id"
	RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateMissing RemoteTemplateNotFoundReason = "source_alias_published_template_missing"
	RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateInvalid RemoteTemplateNotFoundReason = "source_alias_published_template_invalid"
)

// RemoteTemplateNotFoundError reports that no remote template source matched the documented selection rules.
type RemoteTemplateNotFoundError struct {
	RepoURL      string
	RequestedRef string
	ResolvedRef  string
	Reason       RemoteTemplateNotFoundReason
	SourceBranch bool
}

func (err *RemoteTemplateNotFoundError) Error() string {
	switch err.Reason {
	case RemoteTemplateNotFoundReasonSourceAliasMissingPublishOrbitID:
		return fmt.Sprintf(
			"external source branch ref %q from %q is missing source.orbit_id; update .harness/manifest.yaml before installing from the source branch",
			err.RequestedRef,
			err.RepoURL,
		)
	case RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateMissing:
		return fmt.Sprintf(
			"external source branch ref %q from %q resolves to %q, but that published template branch does not exist; run `orbit template publish` first",
			err.RequestedRef,
			err.RepoURL,
			err.ResolvedRef,
		)
	case RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateInvalid:
		return fmt.Sprintf(
			"external source branch ref %q from %q resolves to %q, but that branch is not a valid orbit template branch",
			err.RequestedRef,
			err.RepoURL,
			err.ResolvedRef,
		)
	}

	if strings.TrimSpace(err.RequestedRef) != "" {
		if err.SourceBranch {
			return fmt.Sprintf(
				"external template ref %q from %q is a source branch, not a valid template branch; run `orbit template publish` first",
				err.RequestedRef,
				err.RepoURL,
			)
		}
		return fmt.Sprintf("external template ref %q from %q is not a valid template branch", err.RequestedRef, err.RepoURL)
	}

	return fmt.Sprintf("no valid external template branches found in %q", err.RepoURL)
}

// RemoteTemplateAmbiguityError reports multiple valid remote template sources with no unique winner.
type RemoteTemplateAmbiguityError struct {
	RepoURL    string
	Candidates []RemoteTemplateCandidate
}

func (err *RemoteTemplateAmbiguityError) Error() string {
	return fmt.Sprintf("external template source %q is ambiguous; candidates: %s", err.RepoURL, strings.Join(err.BranchNames(), ", "))
}

// BranchNames returns the stable candidate branch list for user-facing ambiguity output.
func (err *RemoteTemplateAmbiguityError) BranchNames() []string {
	names := make([]string, 0, len(err.Candidates))
	for _, candidate := range err.Candidates {
		names = append(names, candidate.Branch)
	}

	return names
}

// ResolveRemoteTemplateSource enumerates remote template branches and selects one candidate by the documented precedence.
func ResolveRemoteTemplateSource(ctx context.Context, repoRoot string, remoteURL string, requestedRef string) (RemoteTemplateCandidate, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	trimmedRequestedRef := strings.TrimSpace(requestedRef)
	if trimmedRequestedRef != "" {
		candidates, err := EnumerateRemoteTemplateSources(ctx, repoRoot, remoteURL)
		if err != nil {
			return RemoteTemplateCandidate{}, err
		}

		selected, err := selectRemoteTemplateCandidate(trimmedURL, trimmedRequestedRef, candidates)
		if err == nil {
			selected.RequestedRef = trimmedRequestedRef
			selected.ResolutionKind = RemoteTemplateResolutionTemplateBranch
			return selected, nil
		}

		var notFoundErr *RemoteTemplateNotFoundError
		if !errors.As(err, &notFoundErr) {
			return RemoteTemplateCandidate{}, err
		}

		aliased, aliasErr := resolveRemoteTemplateSourceAlias(ctx, repoRoot, trimmedURL, trimmedRequestedRef, trimmedRequestedRef)
		if aliasErr == nil {
			return aliased, nil
		}
		var aliasNotFoundErr *RemoteTemplateNotFoundError
		if errors.As(aliasErr, &aliasNotFoundErr) && aliasNotFoundErr.SourceBranch {
			return RemoteTemplateCandidate{}, aliasNotFoundErr
		}
		return RemoteTemplateCandidate{}, err
	}

	defaultBranch, resolveErr := gitpkg.ResolveRemoteDefaultBranch(ctx, repoRoot, trimmedURL)
	if resolveErr == nil {
		aliased, aliasErr := resolveRemoteTemplateSourceAlias(ctx, repoRoot, trimmedURL, defaultBranch.Ref, defaultBranch.Name)
		if aliasErr == nil {
			return aliased, nil
		}
		var aliasNotFoundErr *RemoteTemplateNotFoundError
		if errors.As(aliasErr, &aliasNotFoundErr) && aliasNotFoundErr.SourceBranch {
			return RemoteTemplateCandidate{}, aliasNotFoundErr
		}
	}

	candidates, err := EnumerateRemoteTemplateSources(ctx, repoRoot, remoteURL)
	if err != nil {
		return RemoteTemplateCandidate{}, err
	}

	selected, err := selectRemoteTemplateCandidate(trimmedURL, "", candidates)
	if err != nil {
		return RemoteTemplateCandidate{}, err
	}

	selected.RequestedRef = ""
	selected.ResolutionKind = RemoteTemplateResolutionTemplateBranch
	return selected, nil
}

// EnumerateRemoteTemplateSources lists remote heads and keeps only those with a valid template manifest.
func EnumerateRemoteTemplateSources(ctx context.Context, repoRoot string, remoteURL string) ([]RemoteTemplateCandidate, error) {
	heads, err := gitpkg.ListRemoteHeads(ctx, repoRoot, remoteURL)
	if err != nil {
		return nil, fmt.Errorf("enumerate remote heads: %w", err)
	}

	candidates := make([]RemoteTemplateCandidate, 0, len(heads))
	for _, head := range heads {
		branchManifestData, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, remoteURL, head.Ref, branchManifestPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("read remote template branch manifest from %q: %w", head.Ref, err)
		}
		branchManifest, err := parseOrbitTemplateBranchManifestData(branchManifestData)
		if err != nil {
			continue
		}
		if branchManifest.Kind != "orbit_template" || strings.TrimSpace(branchManifest.Template.OrbitID) == "" {
			continue
		}

		manifest, err := loadRemoteTemplateCandidateManifest(head.Ref, branchManifest)
		if err != nil {
			continue
		}

		candidates = append(candidates, RemoteTemplateCandidate{
			RepoURL:        strings.TrimSpace(remoteURL),
			Branch:         head.Name,
			Ref:            head.Ref,
			Commit:         head.Commit,
			RequestedRef:   "",
			ResolutionKind: "",
			Manifest:       manifest,
		})
	}

	return candidates, nil
}

func loadRemoteTemplateCandidateManifest(ref string, branchManifest orbitTemplateBranchManifest) (Manifest, error) {
	manifest, err := manifestFromOrbitTemplateBranchManifest(branchManifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("template source %q is not a valid orbit template branch: %w", ref, err)
	}

	return manifest, nil
}

// ResolveRemoteTemplateCandidateSnapshot fetches one chosen remote template branch and materializes its validated template tree.
func ResolveRemoteTemplateCandidateSnapshot(ctx context.Context, repoRoot string, candidate RemoteTemplateCandidate) (LocalTemplateSource, error) {
	var source LocalTemplateSource
	if err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, candidate.RepoURL, candidate.Ref, func(tempRef string) error {
		resolved, err := loadTemplateSourceAtRevision(ctx, repoRoot, tempRef, candidate.Branch, candidate.Commit)
		if err != nil {
			return err
		}
		source = resolved
		return nil
	}); err != nil {
		return LocalTemplateSource{}, fmt.Errorf("resolve external template source %q from %q: %w", candidate.Branch, candidate.RepoURL, err)
	}

	return source, nil
}

func resolveRemoteTemplateSourceSnapshot(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	requestedRef string,
) (RemoteTemplateCandidate, LocalTemplateSource, error) {
	trimmedURL := strings.TrimSpace(remoteURL)
	trimmedRef := strings.TrimSpace(requestedRef)
	if trimmedRef == "" {
		candidate, err := ResolveRemoteTemplateSource(ctx, repoRoot, trimmedURL, "")
		if err != nil {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, err
		}

		source, err := ResolveRemoteTemplateCandidateSnapshot(ctx, repoRoot, candidate)
		if err != nil {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, err
		}

		return candidate, source, nil
	}

	candidate, source, err := resolveExplicitRemoteTemplateSourceSnapshot(ctx, repoRoot, trimmedURL, trimmedRef)
	if err != nil {
		var notFoundErr *RemoteTemplateNotFoundError
		if !errors.As(err, &notFoundErr) {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, err
		}

		candidate, err = ResolveRemoteTemplateSource(ctx, repoRoot, trimmedURL, trimmedRef)
		if err != nil {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, err
		}
		source, err = ResolveRemoteTemplateCandidateSnapshot(ctx, repoRoot, candidate)
		if err != nil {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, err
		}
	}

	return candidate, source, nil
}

func normalizeRemoteRequestedRef(requestedRef string) (branch string, ref string) {
	trimmedRef := strings.TrimSpace(requestedRef)
	if strings.HasPrefix(trimmedRef, "refs/heads/") {
		return strings.TrimPrefix(trimmedRef, "refs/heads/"), trimmedRef
	}

	return trimmedRef, "refs/heads/" + trimmedRef
}

func isMissingRemoteRefError(err error) bool {
	message := err.Error()
	return strings.Contains(message, "couldn't find remote ref") ||
		strings.Contains(message, "invalid refspec")
}

func readOrbitTemplateBranchManifestAtRevision(
	ctx context.Context,
	repoRoot string,
	revision string,
	displayRef string,
) (orbitTemplateBranchManifest, bool, error) {
	exists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, revision, branchManifestPath)
	if err != nil {
		return orbitTemplateBranchManifest{}, false, fmt.Errorf("check orbit template branch manifest at %q: %w", displayRef, err)
	}
	if !exists {
		return orbitTemplateBranchManifest{}, false, nil
	}

	manifestData, err := gitpkg.ReadFileAtRev(ctx, repoRoot, revision, branchManifestPath)
	if err != nil {
		return orbitTemplateBranchManifest{}, false, fmt.Errorf("read orbit template branch manifest at %q: %w", displayRef, err)
	}

	manifest, valid := func() (orbitTemplateBranchManifest, bool) {
		parsedManifest, parseErr := parseOrbitTemplateBranchManifestData(manifestData)
		if parseErr != nil {
			return orbitTemplateBranchManifest{}, false
		}
		if parsedManifest.Kind != "orbit_template" || strings.TrimSpace(parsedManifest.Template.OrbitID) == "" {
			return orbitTemplateBranchManifest{}, false
		}
		return parsedManifest, true
	}()
	if !valid {
		return orbitTemplateBranchManifest{}, false, nil
	}

	return manifest, true, nil
}

func resolveExplicitRemoteTemplateSourceSnapshot(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	requestedRef string,
) (RemoteTemplateCandidate, LocalTemplateSource, error) {
	branchName, fullRef := normalizeRemoteRequestedRef(requestedRef)
	var manifest Manifest
	var source LocalTemplateSource
	invalidTemplate := false

	if err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, remoteURL, requestedRef, func(tempRef string) error {
		_, ok, err := readOrbitTemplateBranchManifestAtRevision(ctx, repoRoot, tempRef, requestedRef)
		if err != nil {
			return err
		}
		if !ok {
			invalidTemplate = true
			return nil
		}

		resolved, err := loadTemplateSourceAtRevision(ctx, repoRoot, tempRef, branchName, "")
		if err != nil {
			return err
		}
		source = resolved
		manifest = resolved.Manifest
		return nil
	}); err != nil {
		if isMissingRemoteRefError(err) {
			return RemoteTemplateCandidate{}, LocalTemplateSource{}, &RemoteTemplateNotFoundError{
				RepoURL:      remoteURL,
				RequestedRef: requestedRef,
			}
		}
		return RemoteTemplateCandidate{}, LocalTemplateSource{}, fmt.Errorf(
			"resolve external template source %q from %q: %w",
			requestedRef,
			remoteURL,
			err,
		)
	}
	if invalidTemplate {
		return RemoteTemplateCandidate{}, LocalTemplateSource{}, &RemoteTemplateNotFoundError{
			RepoURL:      remoteURL,
			RequestedRef: requestedRef,
		}
	}

	return RemoteTemplateCandidate{
		RepoURL:        strings.TrimSpace(remoteURL),
		Branch:         branchName,
		Ref:            fullRef,
		Commit:         source.Commit,
		RequestedRef:   requestedRef,
		ResolutionKind: RemoteTemplateResolutionTemplateBranch,
		Manifest:       manifest,
	}, source, nil
}

func resolveRemoteTemplateSourceAlias(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	sourceRef string,
	requestedRef string,
) (RemoteTemplateCandidate, error) {
	_, manifest, ok, err := inspectRemoteSourceBranch(ctx, repoRoot, remoteURL, sourceRef)
	if err != nil {
		return RemoteTemplateCandidate{}, err
	}
	if !ok {
		return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
			RepoURL:      remoteURL,
			RequestedRef: requestedRef,
		}
	}

	if manifest.Publish == nil || strings.TrimSpace(manifest.Publish.OrbitID) == "" {
		return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
			RepoURL:      remoteURL,
			RequestedRef: requestedRef,
			SourceBranch: true,
			Reason:       RemoteTemplateNotFoundReasonSourceAliasMissingPublishOrbitID,
		}
	}

	resolvedBranch := fmt.Sprintf("orbit-template/%s", manifest.Publish.OrbitID)
	resolvedRef := "refs/heads/" + resolvedBranch
	candidate, err := resolvePublishedRemoteTemplateCandidate(ctx, repoRoot, remoteURL, resolvedRef)
	if err != nil {
		var notFoundErr *RemoteTemplateNotFoundError
		if errors.As(err, &notFoundErr) {
			notFoundErr.RequestedRef = requestedRef
			notFoundErr.SourceBranch = true
			notFoundErr.ResolvedRef = resolvedBranch
		}
		return RemoteTemplateCandidate{}, err
	}
	candidate.RequestedRef = requestedRef
	candidate.ResolutionKind = RemoteTemplateResolutionSourceAlias
	candidate.Branch = resolvedBranch
	candidate.Ref = resolvedRef

	return candidate, nil
}

func inspectRemoteSourceBranch(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	remoteRef string,
) (string, SourceManifest, bool, error) {
	branchName, _ := normalizeRemoteRequestedRef(remoteRef)
	var manifest SourceManifest
	var ok bool

	err := gitpkg.WithFetchedRemoteRef(ctx, repoRoot, remoteURL, remoteRef, func(tempRef string) error {
		exists, err := gitpkg.PathExistsAtRev(ctx, repoRoot, tempRef, sourceManifestRelativePath)
		if err != nil {
			return fmt.Errorf("check %s at %s: %w", sourceManifestRelativePath, remoteRef, err)
		}
		if !exists {
			return nil
		}

		data, err := gitpkg.ReadFileAtRev(ctx, repoRoot, tempRef, sourceManifestRelativePath)
		if err != nil {
			return fmt.Errorf("read %s at %s: %w", sourceManifestRelativePath, remoteRef, err)
		}
		parsed, parseErr := ParseSourceManifestData(data)
		if parseErr == nil && parsed.SourceBranch == branchName {
			manifest = parsed
			ok = true
		}
		return nil
	})
	if err != nil {
		if isMissingRemoteRefError(err) {
			return "", SourceManifest{}, false, nil
		}
		return "", SourceManifest{}, false, fmt.Errorf("inspect remote source marker %q from %q: %w", remoteRef, remoteURL, err)
	}

	return branchName, manifest, ok, nil
}

func resolvePublishedRemoteTemplateCandidate(
	ctx context.Context,
	repoRoot string,
	remoteURL string,
	remoteRef string,
) (RemoteTemplateCandidate, error) {
	heads, err := gitpkg.ListRemoteHeads(ctx, repoRoot, remoteURL)
	if err != nil {
		return RemoteTemplateCandidate{}, fmt.Errorf("enumerate remote heads: %w", err)
	}

	branchName, fullRef := normalizeRemoteRequestedRef(remoteRef)
	for _, head := range heads {
		if head.Ref != fullRef {
			continue
		}

		branchManifestData, err := gitpkg.ReadFileAtRemoteRef(ctx, repoRoot, remoteURL, fullRef, branchManifestPath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
					RepoURL: remoteURL,
					Reason:  RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateInvalid,
				}
			}
			return RemoteTemplateCandidate{}, fmt.Errorf("read remote orbit template branch manifest from %q: %w", fullRef, err)
		}

		branchManifest, parseErr := parseOrbitTemplateBranchManifestData(branchManifestData)
		if parseErr != nil || branchManifest.Kind != "orbit_template" || strings.TrimSpace(branchManifest.Template.OrbitID) == "" {
			return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
				RepoURL: remoteURL,
				Reason:  RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateInvalid,
			}
		}
		manifest, err := loadRemoteTemplateCandidateManifest(fullRef, branchManifest)
		if err != nil {
			return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
				RepoURL: remoteURL,
				Reason:  RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateInvalid,
			}
		}

		return RemoteTemplateCandidate{
			RepoURL:        strings.TrimSpace(remoteURL),
			Branch:         branchName,
			Ref:            fullRef,
			Commit:         head.Commit,
			RequestedRef:   "",
			ResolutionKind: RemoteTemplateResolutionTemplateBranch,
			Manifest:       manifest,
		}, nil
	}

	return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
		RepoURL: remoteURL,
		Reason:  RemoteTemplateNotFoundReasonSourceAliasPublishedTemplateMissing,
	}
}

func selectRemoteTemplateCandidate(remoteURL string, requestedRef string, candidates []RemoteTemplateCandidate) (RemoteTemplateCandidate, error) {
	trimmedRef := strings.TrimSpace(requestedRef)
	if trimmedRef != "" {
		for _, candidate := range candidates {
			if candidate.Branch == trimmedRef || candidate.Ref == trimmedRef {
				return candidate, nil
			}
		}

		return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{
			RepoURL:      remoteURL,
			RequestedRef: trimmedRef,
		}
	}

	switch len(candidates) {
	case 0:
		return RemoteTemplateCandidate{}, &RemoteTemplateNotFoundError{RepoURL: remoteURL}
	case 1:
		return candidates[0], nil
	}

	defaultCandidates := make([]RemoteTemplateCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.Manifest.Template.DefaultTemplate {
			defaultCandidates = append(defaultCandidates, candidate)
		}
	}

	if len(defaultCandidates) == 1 {
		return defaultCandidates[0], nil
	}

	return RemoteTemplateCandidate{}, &RemoteTemplateAmbiguityError{
		RepoURL:    remoteURL,
		Candidates: slices.Clone(candidates),
	}
}
