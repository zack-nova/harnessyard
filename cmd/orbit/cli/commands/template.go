package commands

import "github.com/spf13/cobra"

// NewTemplateCommand creates the Phase 2 template command tree.
func NewTemplateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "template",
		Short: "Manage Orbit template authoring commands",
		Args:  cobra.NoArgs,
	}

	cmd.AddCommand(
		NewTemplateCreateCommand(),
		NewTemplateInitCommand(),
		NewTemplateInitSourceCommand(),
		NewTemplatePublishCommand(),
		NewTemplateSaveCommand(),
		NewTemplateApplyCommand(),
	)

	return cmd
}
