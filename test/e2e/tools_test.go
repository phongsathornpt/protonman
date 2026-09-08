package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EFileAndProcessTools(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	// 1. read_file
	readRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt"}`},
		dir:  ws,
		env:  env,
	})
	if readRes.exitCode != 0 || !strings.Contains(readRes.stdout, "Hello Coding E2E") {
		t.Fatalf("read_file failed (code %d): %s\n%s", readRes.exitCode, readRes.stdout, readRes.stderr)
	}

	// 2. write_file
	writeRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call write_file {"file_path":"created.txt", "content":"Brand New Content"}`},
		dir:  ws,
		env:  env,
	})
	if writeRes.exitCode != 0 {
		t.Fatalf("write_file failed (code %d): %s\n%s", writeRes.exitCode, writeRes.stdout, writeRes.stderr)
	}
	createdDisk, err := os.ReadFile(filepath.Join(ws, "created.txt"))
	if err != nil {
		t.Fatalf("created.txt was not written to disk: %v", err)
	}
	if string(createdDisk) != "Brand New Content" {
		t.Fatalf("created.txt content = %q, want 'Brand New Content'", string(createdDisk))
	}

	// 3. search_replace
	srRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call search_replace {"file_path":"created.txt", "old_string":"Brand New", "new_string":"Updated"}`},
		dir:  ws,
		env:  env,
	})
	if srRes.exitCode != 0 {
		t.Fatalf("search_replace failed (code %d): %s\n%s", srRes.exitCode, srRes.stdout, srRes.stderr)
	}
	updatedDisk, err := os.ReadFile(filepath.Join(ws, "created.txt"))
	if err != nil {
		t.Fatalf("read created.txt after search_replace: %v", err)
	}
	if string(updatedDisk) != "Updated Content" {
		t.Fatalf("updated disk content = %q, want 'Updated Content'", string(updatedDisk))
	}

	// 4. grep
	grepRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call grep {"pattern":"Updated"}`},
		dir:  ws,
		env:  env,
	})
	if grepRes.exitCode != 0 || !strings.Contains(grepRes.stdout, "created.txt") {
		t.Fatalf("grep failed (code %d): %s\n%s", grepRes.exitCode, grepRes.stdout, grepRes.stderr)
	}

	// 5. list_dir
	listRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call list_dir {"path":"."}`},
		dir:  ws,
		env:  env,
	})
	if listRes.exitCode != 0 || !strings.Contains(listRes.stdout, "hello.txt") || !strings.Contains(listRes.stdout, "created.txt") {
		t.Fatalf("list_dir failed (code %d): %s\n%s", listRes.exitCode, listRes.stdout, listRes.stderr)
	}

	// 6. bash
	bashRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"echo 'proton-bash-verified'"}`},
		dir:  ws,
		env:  env,
	})
	if bashRes.exitCode != 0 || !strings.Contains(bashRes.stdout, "proton-bash-verified") {
		t.Fatalf("bash failed (code %d): %s\n%s", bashRes.exitCode, bashRes.stdout, bashRes.stderr)
	}

	// 7. git_status
	gitRes := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call git_status {}`},
		dir:  ws,
		env:  env,
	})
	if gitRes.exitCode != 0 {
		t.Fatalf("git_status failed (code %d): %s\n%s", gitRes.exitCode, gitRes.stdout, gitRes.stderr)
	}
	if !strings.Contains(gitRes.stdout, "Untracked files") && !strings.Contains(gitRes.stdout, "hello.txt") {
		t.Fatalf("git_status output unexpected: %s", gitRes.stdout)
	}
}

func TestE2EApplyPatch(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTONMAN_HOME=" + home}

	patch := "*** Begin Patch\n" +
		"*** Update File: hello.txt\n" +
		"@@\n" +
		"-Hello Coding E2E\n" +
		"+Hello Patched E2E\n" +
		" Line 2\n" +
		"*** End Patch"

	patchEscaped := strings.ReplaceAll(patch, "\n", `\n`)
	prompt := `/call apply_patch {"patch":"` + patchEscaped + `"}`

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", prompt},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 {
		t.Fatalf("apply_patch failed (code %d): %s\n%s", res.exitCode, res.stdout, res.stderr)
	}

	content, err := os.ReadFile(filepath.Join(ws, "hello.txt"))
	if err != nil {
		t.Fatalf("read hello.txt: %v", err)
	}
	if !strings.Contains(string(content), "Hello Patched E2E") {
		t.Fatalf("patch not applied, content = %q", string(content))
	}
}

func TestE2EWorkspaceEscapeRejection(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"../../../../etc/passwd"}`},
		dir:  ws,
		env:  []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected path traversal to fail, got exit 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "outside_workspace") && !strings.Contains(combined, "outside workspace") {
		t.Fatalf("expected outside_workspace error, got: %s", combined)
	}
}

func TestE2EProtectedPathsRejection(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// Create project-local config with protected paths
	projectProton := filepath.Join(ws, ".protonman")
	if err := os.MkdirAll(projectProton, 0o755); err != nil {
		t.Fatalf("mkdir .protonman: %v", err)
	}
	configContent := `
[workspace]
protected_paths = [".env", "secrets/*"]
`
	if err := os.WriteFile(filepath.Join(projectProton, "config.toml"), []byte(configContent), 0o644); err != nil {
		t.Fatalf("write config.toml: %v", err)
	}

	// Create .env in workspace
	if err := os.WriteFile(filepath.Join(ws, ".env"), []byte("SECRET_KEY=12345"), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}

	// With PROTON_TRUST_PROJECT=1, read_file on .env should fail with protected_path
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":".env"}`},
		dir:  ws,
		env: []string{
			"PROTONMAN_HOME=" + home,
			"PROTON_TRUST_PROJECT=1",
		},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected protected path read to fail, got exit 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "protected_path") && !strings.Contains(combined, "protected") {
		t.Fatalf("expected protected_path error, got: %s", combined)
	}
}
