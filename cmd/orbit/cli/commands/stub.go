package commands

import (
	"fmt"

	"github.com/spf13/cobra"
)

//nolint:unused // Retained as a local helper for future command bring-up during MVP phases.
func newStubCommand(use, short, commandName string, args cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(*cobra.Command, []string) error {
			return fmt.Errorf("%s is not implemented yet", commandName)
		},
	}
}
