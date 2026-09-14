//go:build desktop

package desktop

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"fyne.io/fyne/v2"
)

const (
	nerdFontVersion  = "3.5.1"
	nerdFontFilename = "SymbolsNerdFontMono-Regular.ttf"
	nerdFontEnv      = "PROTONMAN_NERD_FONT"
)

var (
	nerdFontOnce     sync.Once
	nerdFontResource fyne.Resource
)

func loadNerdFontResource() fyne.Resource {
	nerdFontOnce.Do(func() {
		path := resolveNerdFontPath()
		if path == "" {
			return
		}
		data, err := os.ReadFile(path)
		if err != nil || len(data) == 0 {
			return
		}
		nerdFontResource = fyne.NewStaticResource(nerdFontFilename, data)
	})
	return nerdFontResource
}

func resolveNerdFontPath() string {
	executable, _ := os.Executable()
	return resolveNerdFontPathFor(os.Getenv(nerdFontEnv), executable, func(path string) bool {
		info, err := os.Stat(path)
		return err == nil && !info.IsDir()
	})
}

func resolveNerdFontPathFor(override, executable string, isFile func(string) bool) string {
	if isFile == nil {
		return ""
	}
	if override = strings.TrimSpace(override); override != "" && isFile(override) {
		return override
	}
	if executable = strings.TrimSpace(executable); executable != "" {
		candidate := filepath.Join(filepath.Dir(executable), "share", "fonts", nerdFontFilename)
		if isFile(candidate) {
			return candidate
		}
	}
	return ""
}
