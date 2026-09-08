package architecture_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProtonmanPublicIdentity(t *testing.T) {
	root := repositoryRoot(t)
	assertFileContains(t, filepath.Join(root, "go.mod"), "module github.com/phongsathornpt/protonman\n")
	assertFileContains(t, filepath.Join(root, "Makefile"), "BIN_NAME := protonman")
	assertFileContains(t, filepath.Join(root, "internal", "base", "buildinfo", "buildinfo.go"), `Name       = "Protonman"`)
	assertFileContains(t, filepath.Join(root, "internal", "base", "buildinfo", "buildinfo.go"), `Repository = "https://github.com/phongsathornpt/protonman"`)

	if info, err := os.Stat(filepath.Join(root, "cmd", "protonman")); err != nil || !info.IsDir() {
		t.Fatalf("canonical cmd/protonman directory missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "cmd", "proton")); !os.IsNotExist(err) {
		t.Fatalf("legacy cmd/proton directory must not exist: %v", err)
	}
}

func TestProtonmanDistributionIdentity(t *testing.T) {
	root := repositoryRoot(t)
	workflow := filepath.Join(root, ".github", "workflows", "release.yml")
	assertFileContains(t, workflow, `package="protonman_${plain_version}_${GOOS}_${GOARCH}"`)
	assertFileContains(t, workflow, `binary="protonman"`)
	assertFileContains(t, workflow, "pattern: protonman-*")
	assertFileContains(t, workflow, "sha256sum protonman_* > checksums.txt")

	body, err := os.ReadFile(workflow)
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	for _, stale := range []string{`package="proton_${plain_version}`, `binary="proton"`, "pattern: proton-*"} {
		if strings.Contains(string(body), stale) {
			t.Errorf("release workflow contains legacy distribution identity %q", stale)
		}
	}
}

func TestProtonmanProjectNamespaceIsCanonical(t *testing.T) {
	root := repositoryRoot(t)
	if _, err := os.Stat(filepath.Join(root, ".protonman", "config.toml")); err != nil {
		t.Fatalf("canonical .protonman/config.toml missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".proton", "config.toml")); !os.IsNotExist(err) {
		t.Fatalf("legacy repo-local .proton/config.toml must not exist: %v", err)
	}
}

func TestLegacyModuleIdentityDoesNotReturn(t *testing.T) {
	root := repositoryRoot(t)
	for _, path := range []string{"go.mod", "Makefile", "README.md", "AGENTS.md", ".github/workflows/release.yml"} {
		body, err := os.ReadFile(filepath.Join(root, path))
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(string(body), "github.com/phongsathornpt/proton/") {
			t.Errorf("%s contains legacy module/repository path", path)
		}
	}
}

func assertFileContains(t *testing.T, path, want string) {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if !strings.Contains(string(body), want) {
		t.Fatalf("%s missing %q", path, want)
	}
}
