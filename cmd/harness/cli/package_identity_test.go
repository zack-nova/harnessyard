package cli_test

import "github.com/zack-nova/harnessyard/cmd/orbit/cli/ids"

func testOrbitPackage(name string) ids.PackageIdentity {
	return ids.PackageIdentity{Type: ids.PackageTypeOrbit, Name: name}
}

func testHarnessPackage(name string) ids.PackageIdentity {
	return ids.PackageIdentity{Type: ids.PackageTypeHarness, Name: name}
}

func testIncludedIn(name string) *ids.PackageIdentity {
	identity := testHarnessPackage(name)
	return &identity
}
