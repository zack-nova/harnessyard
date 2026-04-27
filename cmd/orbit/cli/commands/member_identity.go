package commands

import (
	"errors"
	"fmt"
	"strings"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
)

func resolveMemberIdentityInput(name string, legacyKey string) (string, error) {
	resolvedName := strings.TrimSpace(name)
	resolvedLegacyKey := strings.TrimSpace(legacyKey)

	switch {
	case resolvedName == "" && resolvedLegacyKey == "":
		return "", errors.New(`either --name or --key must be provided`)
	case resolvedName != "" && resolvedLegacyKey != "" && resolvedName != resolvedLegacyKey:
		return "", errors.New(`--name and --key must match when both are provided`)
	case resolvedName == "":
		resolvedName = resolvedLegacyKey
	}

	if err := ids.ValidateOrbitID(resolvedName); err != nil {
		return "", fmt.Errorf("validate member name: %w", err)
	}

	return resolvedName, nil
}
