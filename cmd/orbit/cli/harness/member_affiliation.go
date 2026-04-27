package harness

import (
	"fmt"

	"github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"
	orbittemplate "github.com/zack-nova/harnessyard/cmd/orbit/cli/template"
)

func validateRuntimeMemberAffiliation(index int, source string, ownerHarnessID string, origin *orbittemplate.Source) error {
	return validateRuntimeAffiliationFields(
		fmt.Sprintf("members[%d]", index),
		"owner_harness_id",
		source,
		ownerHarnessID,
		origin,
	)
}

func validateRuntimePackageAffiliation(index int, source string, ownerHarnessID string, origin *orbittemplate.Source) error {
	return validateRuntimeAffiliationFields(
		fmt.Sprintf("packages[%d]", index),
		"included_in",
		source,
		ownerHarnessID,
		origin,
	)
}

func validateRuntimeAffiliationFields(fieldPrefix string, ownerField string, source string, ownerHarnessID string, origin *orbittemplate.Source) error {
	switch source {
	case MemberSourceInstallBundle:
		if ownerHarnessID == "" {
			return fmt.Errorf("%s.%s must be present when source is %q", fieldPrefix, ownerField, source)
		}
	}

	if ownerHarnessID != "" {
		if err := ids.ValidateOrbitID(ownerHarnessID); err != nil {
			nameSuffix := ""
			if ownerField == "included_in" {
				nameSuffix = ".name"
			}
			return fmt.Errorf("%s.%s%s: %w", fieldPrefix, ownerField, nameSuffix, err)
		}
	}
	if origin != nil {
		if err := validateLastStandaloneOrigin(*origin); err != nil {
			return fmt.Errorf("%s.last_standalone_origin.%s", fieldPrefix, err.Error())
		}
	}

	return nil
}

func validateLastStandaloneOrigin(origin orbittemplate.Source) error {
	switch origin.SourceKind {
	case orbittemplate.InstallSourceKindLocalBranch:
	case orbittemplate.InstallSourceKindExternalGit:
		if origin.SourceRepo == "" {
			return fmt.Errorf("source_repo must not be empty for %q", orbittemplate.InstallSourceKindExternalGit)
		}
	default:
		return fmt.Errorf(
			"source_kind must be one of %q or %q",
			orbittemplate.InstallSourceKindLocalBranch,
			orbittemplate.InstallSourceKindExternalGit,
		)
	}
	if origin.SourceRef == "" {
		return fmt.Errorf("source_ref must not be empty")
	}
	if origin.TemplateCommit == "" {
		return fmt.Errorf("template_commit must not be empty")
	}

	return nil
}

func cloneTemplateSource(source *orbittemplate.Source) *orbittemplate.Source {
	if source == nil {
		return nil
	}

	copied := *source
	return &copied
}
