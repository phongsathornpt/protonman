package protonsdk_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestProviderProtocolOwnership(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test file")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), ".."))
	goMod, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(goMod), "github.com/openai/openai-go") {
		t.Fatal("official OpenAI SDK dependency must not return; proton-sdk owns the OpenAI protocol")
	}
	for _, legacy := range []string{"openai_request.go", "openai_stream.go", "openai_official.go", "openai_official_chat.go"} {
		if _, err := os.Stat(filepath.Join(root, "internal", "model", legacy)); err == nil {
			t.Fatalf("legacy protocol implementation returned: internal/model/%s", legacy)
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}
}
