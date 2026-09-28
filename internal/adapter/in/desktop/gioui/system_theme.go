//go:build desktop || desktop_gio

package gioui

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	systemThemeMu      sync.RWMutex
	lastDetectedDark   = true
	lastDetectionTime  time.Time
	detectionTTL       = 1500 * time.Millisecond
	overrideDetectorFn func() bool
)

// isSystemDarkMode returns true if the host operating system is currently using a dark theme.
// Results are cached with a short TTL (1.5s) to avoid unnecessary process execution.
func isSystemDarkMode() bool {
	systemThemeMu.RLock()
	if overrideDetectorFn != nil {
		fn := overrideDetectorFn
		systemThemeMu.RUnlock()
		return fn()
	}
	if time.Since(lastDetectionTime) < detectionTTL {
		val := lastDetectedDark
		systemThemeMu.RUnlock()
		return val
	}
	systemThemeMu.RUnlock()

	dark := queryOSDarkMode()

	systemThemeMu.Lock()
	lastDetectedDark = dark
	lastDetectionTime = time.Now()
	systemThemeMu.Unlock()

	return dark
}

func queryOSDarkMode() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	switch runtime.GOOS {
	case "darwin":
		cmd := exec.CommandContext(ctx, "defaults", "read", "-g", "AppleInterfaceStyle")
		out, err := cmd.Output()
		if err != nil {
			return false // On macOS, absence of AppleInterfaceStyle means Light mode
		}
		return strings.EqualFold(strings.TrimSpace(string(out)), "dark")

	case "windows":
		cmd := exec.CommandContext(ctx, "reg", "query", `HKCU\Software\Microsoft\Windows\CurrentVersion\Themes\Personalize`, "/v", "AppsUseLightTheme")
		out, err := cmd.Output()
		if err == nil {
			// AppsUseLightTheme 0x0 = Dark, 0x1 = Light
			for _, line := range strings.Split(string(out), "\n") {
				if strings.Contains(line, "AppsUseLightTheme") {
					return strings.Contains(line, "0x0")
				}
			}
		}
		return true

	case "linux":
		cmd := exec.CommandContext(ctx, "gsettings", "get", "org.gnome.desktop.interface", "color-scheme")
		out, err := cmd.Output()
		if err == nil {
			s := strings.ToLower(string(out))
			if strings.Contains(s, "dark") {
				return true
			}
			if strings.Contains(s, "light") || strings.Contains(s, "default") {
				return false
			}
		}
		return true

	default:
		return true
	}
}

// resolveThemeMode maps a configured theme identifier (e.g. "system", "dark", "light", "slate-dark", "slate-light")
// to a concrete theme palette ("dark", "light", "slate-dark", "slate-light") supported by newTheme.
func resolveThemeMode(configured string, isDark bool) string {
	configured = strings.ToLower(strings.TrimSpace(configured))
	switch configured {
	case "system", "auto", "device", "":
		if isDark {
			return "dark"
		}
		return "light"
	case "dark", "light", "slate-dark", "slate-light":
		return configured
	default:
		if strings.Contains(configured, "dark") {
			return "dark"
		}
		if strings.Contains(configured, "light") {
			return "light"
		}
		if isDark {
			return "dark"
		}
		return "light"
	}
}
