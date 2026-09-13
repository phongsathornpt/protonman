package architecture_test

import "testing"

func TestInboundAdaptersUseApplicationConversationBoundary(t *testing.T) {
	packages := listPackages(t)
	for _, adapter := range []string{
		modulePath + "/internal/adapter/in/acp",
		modulePath + "/internal/adapter/in/headless",
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertPackageNoImportPrefixes(t, packages, adapter, []string{modulePath + "/internal/engine/turn"})
	}
}

func TestBuiltinToolsDoNotDependOnFeatureSubsystems(t *testing.T) {
	packages := listPackages(t)
	source := modulePath + "/internal/adapter/out/tool/builtin"
	assertNoImports(t, packages, source, []string{"net/http"})
	assertPackageNoImportPrefixes(t, packages, source, []string{
		modulePath + "/internal/feature/agent",
		modulePath + "/internal/base/buildinfo",
		modulePath + "/internal/feature/skill",
		modulePath + "/internal/feature/todo",
	})
}

func TestSessionDomainDoesNotOwnFilesystemPersistence(t *testing.T) {
	packages := listPackages(t)
	source := modulePath + "/internal/core/session"
	assertNoImports(t, packages, source, []string{"os"})
	assertPackageNoImportPrefixes(t, packages, source, []string{modulePath + "/internal/adapter/out/sessionfs"})
}

func TestTUIDoesNotDependOnSessionPersistenceDomain(t *testing.T) {
	packages := listPackages(t)
	for _, pkgPath := range []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertPackageNoImportPrefixes(t, packages, pkgPath, []string{modulePath + "/internal/core/session"})
	}
}

func TestTUIDoesNotDependOnProjectDirectly(t *testing.T) {
	packages := listPackages(t)
	for _, pkgPath := range []string{
		modulePath + "/internal/adapter/in/tui",
		modulePath + "/internal/adapter/in/tui/runtime",
	} {
		assertPackageNoImportPrefixes(t, packages, pkgPath, []string{modulePath + "/internal/feature/project"})
	}
}
