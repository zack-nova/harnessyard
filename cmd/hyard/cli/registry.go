package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	gitpkg "github.com/zack-nova/harnessyard/cmd/orbit/cli/git"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	"github.com/zack-nova/harnessyard/cmd/orbit/cli/registry"
)

func newRegistryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Work with Package Registry entries",
		Long: "Work with Package Registry entries.\n" +
			"Generate reviewable Registry Entry Candidates for catalog-as-code submission.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(newRegistryEntryCommand())

	return cmd
}

func newRegistryEntryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "entry",
		Short: "Generate Registry Entry Candidates",
		Long: "Generate Registry Entry Candidates for published Orbit or Harness Packages.\n" +
			"Candidates validate source evidence and emit YAML for review in a Package Registry checkout.",
		Args: cobra.NoArgs,
	}

	cmd.AddCommand(newRegistryEntryOrbitCommand())
	cmd.AddCommand(newRegistryEntryHarnessCommand())

	return cmd
}

func newRegistryEntryOrbitCommand() *cobra.Command {
	return newRegistryEntryPackageCommand(registryEntryPackageCommandOptions{
		PackageType:  ids.PackageTypeOrbit,
		PackageLabel: "Orbit Package",
		Use:          "orbit <namespace/name@version>",
		Short:        "Generate an Orbit Package Registry Entry Candidate",
		Long: "Generate an Orbit Package Registry Entry Candidate from published Orbit Package evidence.\n" +
			"The command validates the source remote, source ref, resolved commit, package identity,\n" +
			"and installability through the existing Orbit install preview path.",
		Example: "" +
			"  hyard registry entry orbit acme/docs@0.1.0 --source https://example.com/acme/docs.git --ref orbit-template/docs --package docs\n" +
			"  hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs --out candidate.yaml\n" +
			"  hyard registry entry orbit acme/docs@0.1.0 --source origin --ref orbit-template/docs --package docs --registry ../hyard-registry\n",
		BuildCandidate: registry.BuildOrbitEntryCandidate,
	})
}

func newRegistryEntryHarnessCommand() *cobra.Command {
	return newRegistryEntryPackageCommand(registryEntryPackageCommandOptions{
		PackageType:  ids.PackageTypeHarness,
		PackageLabel: "Harness Package",
		Use:          "harness <namespace/name@version>",
		Short:        "Generate a Harness Package Registry Entry Candidate",
		Long: "Generate a Harness Package Registry Entry Candidate from published Harness Package evidence.\n" +
			"The command validates the source remote, source ref, resolved commit, package identity,\n" +
			"and installability through the existing Harness install preview path.",
		Example: "" +
			"  hyard registry entry harness acme/workspace@0.1.0 --source https://example.com/acme/workspace.git --ref harness-template/workspace --package workspace\n" +
			"  hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace --out candidate.yaml\n" +
			"  hyard registry entry harness acme/workspace@0.1.0 --source origin --ref harness-template/workspace --package workspace --registry ../hyard-registry\n",
		BuildCandidate: registry.BuildHarnessEntryCandidate,
	})
}

type registryEntryPackageCommandOptions struct {
	PackageType    string
	PackageLabel   string
	Use            string
	Short          string
	Long           string
	Example        string
	BuildCandidate func(cmdContext context.Context, input registry.EntryCandidateInput) (registry.EntryCandidate, error)
}

func newRegistryEntryPackageCommand(options registryEntryPackageCommandOptions) *cobra.Command {
	var sourceRemote string
	var sourceRef string
	var expectedCommit string
	var packageIdentity string
	var targetPath string
	var packageStatus string
	var outPath string
	var registryRoot string

	cmd := &cobra.Command{
		Use:     options.Use,
		Short:   options.Short,
		Long:    options.Long,
		Example: options.Example,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			workingDir, err := hyardWorkingDirFromCommand(cmd)
			if err != nil {
				return err
			}
			repo, err := gitpkg.DiscoverRepo(cmd.Context(), workingDir)
			if err != nil {
				return fmt.Errorf("discover git repository: %w", err)
			}
			status, err := registryPackageStatusFromFlag(packageStatus)
			if err != nil {
				return err
			}
			candidate, err := options.BuildCandidate(cmd.Context(), registry.EntryCandidateInput{
				RepoRoot:        repo.Root,
				Coordinate:      args[0],
				PackageType:     options.PackageType,
				PackageIdentity: packageIdentity,
				PackageStatus:   status,
				SourceRemote:    sourceRemote,
				SourceRef:       sourceRef,
				ExpectedCommit:  expectedCommit,
				TargetPath:      targetPath,
			})
			if err != nil {
				return fmt.Errorf("build %s registry entry candidate: %w", options.PackageType, err)
			}
			data, err := registry.MarshalEntryCandidate(candidate)
			if err != nil {
				return fmt.Errorf("marshal %s registry entry candidate: %w", options.PackageType, err)
			}

			return emitRegistryEntryCandidate(cmd, workingDir, candidate, data, outPath, registryRoot)
		},
	}

	cmd.Flags().StringVar(&sourceRemote, "source", "", "Source Git remote containing the published "+options.PackageLabel+"; omit only for local preview output")
	cmd.Flags().StringVar(&sourceRef, "ref", "", "Source Git ref for the published "+options.PackageLabel)
	cmd.Flags().StringVar(&expectedCommit, "commit", "", "Optional expected source commit SHA to verify")
	cmd.Flags().StringVar(&packageIdentity, "package", "", options.PackageLabel+" identity name or SemVer package coordinate")
	cmd.Flags().StringVar(&targetPath, "target", "", "Registry-relative target path; defaults to packages/<namespace>/index.yaml")
	cmd.Flags().StringVar(&packageStatus, "status", string(registry.PackageStatusActive), "Registry package status for the candidate")
	cmd.Flags().StringVar(&outPath, "out", "", "Write the YAML candidate to a chosen file")
	cmd.Flags().StringVar(&registryRoot, "registry", "", "Write the YAML candidate under a local registry checkout at its target path")

	return cmd
}

func registryPackageStatusFromFlag(raw string) (registry.PackageStatus, error) {
	status := registry.PackageStatus(strings.ToLower(strings.TrimSpace(raw)))
	switch status {
	case "", registry.PackageStatusActive:
		return registry.PackageStatusActive, nil
	case registry.PackageStatusDeprecated, registry.PackageStatusYanked, registry.PackageStatusBlocked:
		return status, nil
	default:
		return "", fmt.Errorf("registry package status must be %q, %q, %q, or %q", registry.PackageStatusActive, registry.PackageStatusDeprecated, registry.PackageStatusYanked, registry.PackageStatusBlocked)
	}
}

func emitRegistryEntryCandidate(
	cmd *cobra.Command,
	workingDir string,
	candidate registry.EntryCandidate,
	data []byte,
	outPath string,
	registryRoot string,
) error {
	if strings.TrimSpace(outPath) != "" && strings.TrimSpace(registryRoot) != "" {
		return fmt.Errorf("--out and --registry cannot be used together")
	}
	if strings.TrimSpace(outPath) == "" && strings.TrimSpace(registryRoot) == "" {
		if _, err := cmd.OutOrStdout().Write(data); err != nil {
			return fmt.Errorf("write registry entry candidate to stdout: %w", err)
		}
		return nil
	}
	if !candidate.Submittable {
		return fmt.Errorf("local-only registry entry previews cannot be written with --out or --registry; provide --source for a submittable candidate")
	}

	filename := strings.TrimSpace(outPath)
	if filename == "" {
		root := strings.TrimSpace(registryRoot)
		if !filepath.IsAbs(root) {
			root = filepath.Join(workingDir, root)
		}
		filename = filepath.Join(root, filepath.FromSlash(candidate.TargetPath))
	} else if !filepath.IsAbs(filename) {
		filename = filepath.Join(workingDir, filename)
	}

	if err := registry.WriteEntryCandidate(filename, data); err != nil {
		return fmt.Errorf("write registry entry candidate: %w", err)
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "wrote registry entry candidate %s\n", filepath.Clean(filename)); err != nil {
		return fmt.Errorf("write registry entry candidate result: %w", err)
	}
	return nil
}
