package architecture_test

import "testing"

type layerContract struct {
	name              string
	packagePrefix     string
	forbiddenPrefixes []string
}

func TestLayerDependencyContract(t *testing.T) {
	packages := listPackages(t)
	contracts := []layerContract{
		{
			name:          "proton-sdk is independent from CLI internals",
			packagePrefix: modulePath + "/pkg/proton-sdk",
			forbiddenPrefixes: []string{
				modulePath + "/internal",
				modulePath + "/cmd",
			},
		},
		{
			name:          "base is the innermost internal layer",
			packagePrefix: modulePath + "/internal/base",
			forbiddenPrefixes: []string{
				modulePath + "/internal",
				modulePath + "/cmd",
			},
		},
		{
			name:          "core does not depend on outer application layers",
			packagePrefix: modulePath + "/internal/core",
			forbiddenPrefixes: []string{
				modulePath + "/internal/app",
				modulePath + "/internal/engine",
				modulePath + "/internal/feature",
				modulePath + "/internal/adapter",
				modulePath + "/cmd",
			},
		},
		{
			name:          "application does not depend on concrete adapters",
			packagePrefix: modulePath + "/internal/app",
			forbiddenPrefixes: []string{
				modulePath + "/internal/adapter",
				modulePath + "/cmd",
			},
		},
		{
			name:          "OpenAI provider depends on SDK contracts, not the use cases",
			packagePrefix: modulePath + "/pkg/proton-sdk/provider/openai",
			forbiddenPrefixes: []string{
				modulePath + "/pkg/proton-sdk/usecase",
			},
		},
		{
			name:          "Anthropic provider depends on SDK contracts, not the use cases",
			packagePrefix: modulePath + "/pkg/proton-sdk/provider/anthropic",
			forbiddenPrefixes: []string{
				modulePath + "/pkg/proton-sdk/usecase",
			},
		},
	}

	for _, contract := range contracts {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			assertNoImportPrefixes(t, packages, contract.packagePrefix, contract.forbiddenPrefixes)
		})
	}
}

// TestSDKHasNoFacadePackage pins the facade removal: no package may import the
// former root facade, and the facade package itself must not exist.
func TestSDKHasNoFacadePackage(t *testing.T) {
	packages := listPackages(t)
	for importPath, pkg := range packages {
		if importPath == modulePath+"/pkg/proton-sdk" {
			t.Errorf("facade package %s must not exist; depend on domain, port, or usecase directly", importPath)
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == modulePath+"/pkg/proton-sdk" {
				t.Errorf("package %s imports the removed SDK facade; depend on domain, port, or usecase directly", importPath)
			}
		}
	}
}

func TestSDKLayerDependencyDirection(t *testing.T) {
	packages := listPackages(t)
	assertNoImportPrefixes(t, packages, modulePath+"/pkg/proton-sdk/domain", []string{
		modulePath + "/pkg/proton-sdk/port",
		modulePath + "/pkg/proton-sdk/usecase",
		modulePath + "/pkg/proton-sdk/provider",
		modulePath + "/pkg/proton-sdk/internal",
	})
	assertNoImportPrefixes(t, packages, modulePath+"/pkg/proton-sdk/port", []string{
		modulePath + "/pkg/proton-sdk/usecase",
		modulePath + "/pkg/proton-sdk/provider",
		modulePath + "/pkg/proton-sdk/internal",
	})
	assertNoImportPrefixes(t, packages, modulePath+"/pkg/proton-sdk/usecase", []string{
		modulePath + "/pkg/proton-sdk/provider",
		modulePath + "/pkg/proton-sdk/internal/providerutil",
	})
}
