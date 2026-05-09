package commands

import "github.com/spf13/cobra"

const (
	withSpecOrbitCreateFlagHelp = "Create docs/<orbit-id>.md and docs/<orbit-id>/README.md and add a spec rule member including docs/<orbit-id>.md and docs/<orbit-id>/**"
	withSpecAuthoringFlagHelp   = "When creating the initial orbit, create docs/<orbit-package>.md and docs/<orbit-package>/README.md and add a spec rule member including docs/<orbit-package>.md and docs/<orbit-package>/**"
)

func mustMarkFlagRequired(cmd *cobra.Command, name string) {
	if err := cmd.MarkFlagRequired(name); err != nil {
		panic(err)
	}
}
