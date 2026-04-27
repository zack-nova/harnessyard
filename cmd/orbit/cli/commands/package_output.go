package commands

import "github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"

type packageIdentityJSON = ids.PackageIdentity

func orbitPackageOutput(name string) packageIdentityJSON {
	return packageIdentityJSON{
		Type: ids.PackageTypeOrbit,
		Name: name,
	}
}
