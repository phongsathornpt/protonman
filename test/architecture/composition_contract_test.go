package architecture_test

import "testing"

func TestInternalPackagesDoNotDependOnCompositionRoot(t *testing.T) {
	packages := listPackages(t)
	assertNoImportPrefixes(t, packages, modulePath+"/internal", []string{modulePath + "/cmd"})
}
