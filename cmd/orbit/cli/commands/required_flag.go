package commands

import "fmt"

func errorsNewRequiredFlag(flagName string) error {
	return fmt.Errorf("required flag %q not set", flagName)
}
