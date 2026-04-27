package orbittemplate

import "github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"

func testOrbitPackage(name string) ids.PackageIdentity {
	return ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: name}
}
