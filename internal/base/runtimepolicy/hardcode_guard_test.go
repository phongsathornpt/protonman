package runtimepolicy_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestMutableProductLiteralsStayCentralized(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", "..", ".."))
	allowed := map[string]string{
		"PROTONMAN_HOME":               "internal/base/envconfig/env.go",
		"PROTONMAN_TRUST_PROJECT":      "internal/base/envconfig/env.go",
		"PROTONMAN_SESSION_ID":         "internal/base/envconfig/env.go",
		"PROTONMAN_SANDBOX":            "internal/base/envconfig/env.go",
		"PROTONMAN_TELEMETRY":          "internal/base/envconfig/env.go",
		"PROTONMAN_DEBUG_LOG":          "internal/base/envconfig/env.go",
		"PROTONMAN_FORCE_TTY":          "internal/base/envconfig/env.go",
		"PROTON_TRUST_PROJECT":         "internal/base/envconfig/env.go",
		"PROTON_SESSION_ID":            "internal/base/envconfig/env.go",
		"PROTON_SANDBOX":               "internal/base/envconfig/env.go",
		"PROTON_TELEMETRY":             "internal/base/envconfig/env.go",
		"PROTON_DEBUG_LOG":             "internal/base/envconfig/env.go",
		"PROTON_FORCE_TTY":             "internal/base/envconfig/env.go",
		".protonman":                   "internal/app/appdirs/dirs.go",
		".proton":                      "internal/app/appdirs/dirs.go",
		"~/.protonman/config.toml":     "internal/app/appdirs/dirs.go",
		"~/.protonman/skills/":         "internal/app/appdirs/dirs.go",
		"~/.protonman/logs/mcp/":       "internal/app/appdirs/dirs.go",
		"todo.md":                      "internal/core/session/resources.go",
		"https://protonman.dev/api/v1": "internal/adapter/out/model/provider_preset.go",
		"https://opencode.ai/zen/v1":   "internal/adapter/out/model/provider_preset.go",
		"https://api.openai.com/v1":    "internal/adapter/out/model/provider_preset.go",
		"https://api.anthropic.com":    "internal/adapter/out/model/provider_preset.go",
	}

	for _, dir := range []string{"cmd", "internal"} {
		walkProductionGoFiles(t, filepath.Join(root, dir), func(path string, value string) {
			rel, err := filepath.Rel(root, path)
			if err != nil {
				t.Fatal(err)
			}
			rel = filepath.ToSlash(rel)
			if strings.Contains(value, "Protonman/1.0") {
				t.Errorf("legacy User-Agent hardcode in %s: %q", rel, value)
			}
			if want, guarded := allowed[value]; guarded && rel != want {
				t.Errorf("mutable product literal %q escaped %s into %s", value, want, rel)
			}
		})
	}
}

func walkProductionGoFiles(t *testing.T, root string, visit func(path, value string)) {
	t.Helper()
	set := token.NewFileSet()
	err := filepath.Walk(root, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(set, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			lit, ok := node.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if value, err := strconv.Unquote(lit.Value); err == nil {
				visit(path, value)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
