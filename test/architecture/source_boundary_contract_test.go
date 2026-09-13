package architecture_test

import "testing"

func TestInboundAdaptersDoNotPerformConfigPersistence(t *testing.T) {
	configImport := modulePath + "/internal/adapter/out/config"
	for _, adapter := range []string{
		"internal/adapter/in/tui",
		"internal/adapter/in/acp",
		"internal/adapter/in/headless",
	} {
		assertNoImportedPackageCallPrefixes(
			t,
			adapter,
			configImport,
			[]string{"Load", "Save", "Delete"},
			false,
			adapter+" performs config persistence directly",
		)
	}
}

func TestTUIDoesNotPerformProviderDiscoveryDirectly(t *testing.T) {
	assertNoImportedPackageCallPrefixes(
		t,
		"internal/adapter/in/tui",
		modulePath+"/internal/adapter/out/model",
		[]string{"FetchProviderModels"},
		true,
		"TUI performs provider discovery directly",
	)
}

func TestApplicationDoesNotExposeAgentCoordinatorEscapeHatch(t *testing.T) {
	assertNoMethod(
		t,
		"internal/app",
		"Agents",
		"Coordinator",
		true,
		"application exposes concrete agent coordinator escape hatch",
	)
}
