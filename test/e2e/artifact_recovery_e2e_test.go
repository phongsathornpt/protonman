package e2e_test

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestE2EGeminiStyleImageProbeRecoversToReadFile(t *testing.T) {
	ws := newTestWorkspace(t)
	home := newTestHome(t)
	server := newMockLLMServer(t)
	setupMockModelConfig(t, server, home, "gemini-3.8-flash")
	writeE2ETestPNG(t, filepath.Join(ws, "screen.png"))

	server.AddToolCallResponse(
		"image-probe",
		"bash",
		`{"command":"python3 -c 'from PIL import Image; im=Image.open(\"screen.png\"); print(im.size)'"}`,
	)
	server.AddTextResponse("Image inspection completed through the workspace artifact reader.")

	res := runProton(t, runOptions{
		args: []string{"-y", "-p", "Inspect screen.png and describe its basic visual properties"},
		dir:  ws, env: []string{"PROTONMAN_HOME=" + home},
	})
	if res.exitCode != 0 {
		t.Fatalf("artifact recovery turn failed (code %d): %s %s", res.exitCode, res.stdout, res.stderr)
	}
	if !strings.Contains(res.stdout, "workspace artifact reader") {
		t.Fatalf("stdout missing final response: %s", res.stdout)
	}
	requests := server.Requests()
	if got, want := len(requests), 2; got != want {
		t.Fatalf("model requests = %d, want %d", got, want)
	}
	if !requestMessagesContain(requests[0], "Use read_file for known workspace artifacts") {
		t.Fatalf("Gemini request missing artifact guidance: %#v", requests[0]["messages"])
	}
	if !toolAppearsBefore(requests[0], "read_file", "bash") {
		t.Fatalf("read_file was not published before bash: %#v", requests[0]["tools"])
	}
	for _, want := range []string{`"kind":"image"`, `"edge_density"`, `"ascii_preview"`} {
		if !requestMessagesContain(requests[1], want) {
			t.Fatalf("second request missing recovered image evidence %q: %#v", want, requests[1]["messages"])
		}
	}
	for _, unwanted := range []string{"use_dedicated_tool", `"code":"invalid_arguments"`} {
		if requestMessagesContain(requests[1], unwanted) {
			t.Fatalf("second request leaked intermediate recovery failure %q: %#v", unwanted, requests[1]["messages"])
		}
	}
}

func setupMockModelConfig(t *testing.T, server *mockLLMServer, home, model string) {
	t.Helper()
	config := fmt.Sprintf(`
[model]
default = %q
provider = "protonman"

[providers.protonman]
api_key = "mock-api-key"
base_url = %q
`, model, server.URL())
	path := filepath.Join(home, ".protonman", "config.toml")
	if err := os.WriteFile(path, []byte(config), 0o644); err != nil {
		t.Fatalf("write mock model config: %v", err)
	}
}

func writeE2ETestPNG(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{R: uint8(32 + x*16), G: uint8(48 + y*24), B: 112, A: 255})
		}
	}
	if err := png.Encode(file, img); err != nil {
		t.Fatal(err)
	}
}

func toolAppearsBefore(request map[string]any, first, second string) bool {
	tools, ok := request["tools"].([]any)
	if !ok {
		return false
	}
	firstIndex, secondIndex := -1, -1
	for index, raw := range tools {
		entry, _ := raw.(map[string]any)
		function, _ := entry["function"].(map[string]any)
		name, _ := function["name"].(string)
		if name == first && firstIndex < 0 {
			firstIndex = index
		}
		if name == second && secondIndex < 0 {
			secondIndex = index
		}
	}
	return firstIndex >= 0 && secondIndex >= 0 && firstIndex < secondIndex
}
