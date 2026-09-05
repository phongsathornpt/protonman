package e2e_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EFullBuiltinTools(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	env := []string{"PROTON_HOME=" + home}

	// 1. read_file byte pagination returns a usable continuation offset.
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt","limit":17}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "Hello Proton E2E") || strings.Contains(res.stdout, "Line 2") {
		t.Fatalf("read_file first page failed: %s %s", res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "continue with offset=17") {
		t.Fatalf("read_file first page missing continuation: %s", res.stdout)
	}

	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"hello.txt","offset":17,"limit":7}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "Line 2") || strings.Contains(res.stdout, "Hello Proton E2E") {
		t.Fatalf("read_file continuation failed: %s %s", res.stdout, res.stderr)
	}

	// 2. read_file non-existent file
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call read_file {"path":"non_existent.txt"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode == 0 || (!strings.Contains(res.stdout+res.stderr, "no such file") && !strings.Contains(res.stdout+res.stderr, "not found")) {
		t.Fatalf("expected file not found error, got: %s %s", res.stdout, res.stderr)
	}

	// 3. write_file creating nested directories
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call write_file {"file_path":"nested/deep/dir/file.txt","content":"deeply nested content"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 {
		t.Fatalf("write_file nested failed: %s %s", res.stdout, res.stderr)
	}
	nestedBytes, err := os.ReadFile(filepath.Join(ws, "nested", "deep", "dir", "file.txt"))
	if err != nil || string(nestedBytes) != "deeply nested content" {
		t.Fatalf("nested file not written properly: %v", err)
	}

	// 4. search_replace error on missing string
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call search_replace {"file_path":"hello.txt","old_string":"NonExistentTextXYZ","new_string":"replacement"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "not found") {
		t.Fatalf("expected search_replace string not found error, got: %s %s", res.stdout, res.stderr)
	}

	// 5. bash execution with non-zero exit code
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call bash {"command":"exit 42"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode == 0 || !strings.Contains(res.stdout+res.stderr, "exit status 42") {
		t.Fatalf("expected non-zero bash exit code, got: %s %s", res.stdout, res.stderr)
	}

	// 6. grep with pattern matching
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call grep {"pattern":"deeply"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "nested/deep/dir/file.txt") {
		t.Fatalf("grep failed to find nested file: %s %s", res.stdout, res.stderr)
	}

	// 7. list_dir nested directory
	res = runProton(t, runOptions{
		args: []string{"-y", "-p", `/call list_dir {"path":"nested/deep"}`},
		dir:  ws,
		env:  env,
	})
	if res.exitCode != 0 || !strings.Contains(res.stdout, "dir") {
		t.Fatalf("list_dir nested failed: %s %s", res.stdout, res.stderr)
	}
}

func TestE2EWebFetchTool(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)

	// web_fetch blocks loopback / internal IPs for SSRF defense
	res := runProton(t, runOptions{
		args: []string{"-y", "-p", `/call web_fetch {"url":"http://127.0.0.1:8080/internal-data"}`},
		dir:  ws,
		env:  []string{"PROTON_HOME=" + home},
	})
	if res.exitCode == 0 {
		t.Fatalf("expected loopback web_fetch to fail, got exit 0: %s", res.stdout)
	}
	combined := res.stdout + res.stderr
	if !strings.Contains(combined, "network denied") && !strings.Contains(combined, "loopback") {
		t.Fatalf("expected SSRF loopback rejection, got: %s", combined)
	}
}
