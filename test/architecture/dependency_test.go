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
	}

	for _, contract := range contracts {
		contract := contract
		t.Run(contract.name, func(t *testing.T) {
			assertNoImportPrefixes(t, packages, contract.packagePrefix, contract.forbiddenPrefixes)
		})
	}
}
