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
			packagePrefix: modulePath + "/proton-sdk",
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
			name:          "OpenAI provider depends on SDK contracts, not the facade or use cases",
			packagePrefix: modulePath + "/proton-sdk/provider/openai",
			forbiddenPrefixes: []string{
				modulePath + "/proton-sdk/usecase",
			},
		},
		{
			name:          "Anthropic provider depends on SDK contracts, not the facade or use cases",
			packagePrefix: modulePath + "/proton-sdk/provider/anthropic",
			forbiddenPrefixes: []string{
				modulePath + "/proton-sdk/usecase",
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

func TestProviderAdaptersDoNotImportRootFacade(t *testing.T) {
	packages := listPackages(t)
	for _, provider := range []string{"openai", "anthropic"} {
		assertNoImports(t, packages, modulePath+"/proton-sdk/provider/"+provider, []string{
			modulePath + "/proton-sdk",
		})
	}
}

func TestCoreDoesNotImportSDKFacade(t *testing.T) {
	packages := listPackages(t)
	for importPath, pkg := range packages {
		if !packageWithin(importPath, modulePath+"/internal/core") {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == modulePath+"/proton-sdk" {
				t.Errorf("package %s imports the public SDK facade; use domain or port directly", importPath)
			}
		}
	}
}

func TestApplicationDoesNotImportSDKFacade(t *testing.T) {
	packages := listPackages(t)
	for importPath, pkg := range packages {
		if !packageWithin(importPath, modulePath+"/internal/app") {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == modulePath+"/proton-sdk" {
				t.Errorf("package %s imports the public SDK facade; use domain or port directly", importPath)
			}
		}
	}
}

func TestEngineDoesNotImportSDKFacade(t *testing.T) {
	packages := listPackages(t)
	for importPath, pkg := range packages {
		if !packageWithin(importPath, modulePath+"/internal/engine") {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == modulePath+"/proton-sdk" {
				t.Errorf("package %s imports the public SDK facade; use domain, port, or usecase directly", importPath)
			}
		}
	}
}

func TestProductionCodeDoesNotImportSDKFacade(t *testing.T) {
	packages := listPackages(t)
	for importPath, pkg := range packages {
		if packageWithin(importPath, modulePath+"/proton-sdk") {
			continue
		}
		for _, imported := range pkg.Imports {
			if imported == modulePath+"/proton-sdk" {
				t.Errorf("package %s imports the public SDK facade; depend on domain, port, or usecase directly", importPath)
			}
		}
	}
}

func TestSDKLayerDependencyDirection(t *testing.T) {
	packages := listPackages(t)
	assertNoImportPrefixes(t, packages, modulePath+"/proton-sdk/domain", []string{
		modulePath + "/proton-sdk",
	})
	assertNoImportPrefixes(t, packages, modulePath+"/proton-sdk/port", []string{
		modulePath + "/proton-sdk/usecase",
		modulePath + "/proton-sdk/provider",
	})
	assertNoImportPrefixes(t, packages, modulePath+"/proton-sdk/usecase", []string{
		modulePath + "/proton-sdk/provider",
		modulePath + "/proton-sdk/internal/providerutil",
	})
}
