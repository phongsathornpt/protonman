package architecture_test

import "testing"

func TestMemoryDomainDoesNotDependOnOuterLayers(t *testing.T) {
	packages := listPackages(t)
	assertNoImports(t, packages, modulePath+"/internal/core/memory", []string{
		modulePath + "/cmd/protonman",
		modulePath + "/internal/adapter/out/config",
		modulePath + "/internal/adapter/out/memoryfs",
		modulePath + "/internal/adapter/out/model",
		modulePath + "/internal/adapter/out/sessionfs",
		modulePath + "/internal/app",
		modulePath + "/internal/engine/prompt",
		modulePath + "/internal/engine/toolcall",
		modulePath + "/internal/engine/turn",
		modulePath + "/internal/feature/agent",
		modulePath + "/internal/feature/memory",
		modulePath + "/internal/feature/skill",
	})
}
