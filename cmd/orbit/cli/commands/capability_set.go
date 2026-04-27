package commands

import "github.com/spf13/cobra"

// NewCapabilitySetCommand creates the orbit capability set command tree.
func NewCapabilitySetCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Write hosted orbit capability truth",
		Long:  "Write hosted capability truth using the v0.66 path-scope and remote-URI model.",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		newCapabilitySetCommandsPathsCommand(),
		newCapabilitySetSkillsLocalPathsCommand(),
		newCapabilitySetSkillsRemoteURIsCommand(),
		newCapabilitySetEntryCommand("command"),
		newCapabilitySetEntryCommand("skill"),
	)

	return cmd
}
