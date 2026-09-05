package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EConfigPrecedenceAndWarnings(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// 1. Write global config.toml
	homeProton := filepath.Join(home, ".proton")
	_ = os.MkdirAll(homeProton, 0o755)
	globalTOML := `
[model]
default = "global-model"
provider = "protonman"
`
	if err := os.WriteFile(filepath.Join(homeProton, "config.toml"), []byte(globalTOML), 0o644); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	// 2. Write project local .proton/config.toml (untrusted)
	projectProton := filepath.Join(ws, ".proton")
	_ = os.MkdirAll(projectProton, 0o755)
	projectTOML := `
[model]
default = "project-model"
`
	if err := os.WriteFile(filepath.Join(projectProton, "config.toml"), []byte(projectTOML), 0o644); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	// Without PROTON_TRUST_PROJECT, a warning should be emitted on stderr about untrusted project config
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'untrusted-check'"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("run failed: %s %s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stderr, "warning:") {
		t.Fatalf("expected untrusted project warning on stderr, got: %s", res.stderr)
	}

	// With PROTON_TRUST_PROJECT=1, project config is trusted and loaded
	resTrusted := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'trusted-check'"}`},
		dir:  ws,
		env: []string{
			"PROTON_HOME=" + home,
			"PROTON_TRUST_PROJECT=1",
		},
	})
	if resTrusted.exitCode != 0 {
		t.Fatalf("trusted run failed: %s %s", resTrusted.stdout, resTrusted.stderr)
	}
}

func TestE2EConfigMalformedTOMLHandling(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Write invalid/corrupt TOML syntax
	homeProton := filepath.Join(home, ".proton")
	_ = os.MkdirAll(homeProton, 0o755)
	_ = os.WriteFile(filepath.Join(homeProton, "config.toml"), []byte("[model\nmalformed = syntax {{{"), 0o644)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "test"},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected exit code > 0 on malformed TOML, got 0")
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "load configuration") {
		t.Fatalf("expected load configuration error, got: %s", combined)
	}
}
