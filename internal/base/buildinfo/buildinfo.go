package buildinfo

import (
	"runtime/debug"
	"strings"
)

const (
	Name       = "Proton"
	Repository = "https://github.com/phongsathornpt/protonman"
)

// version may be overridden at build time with -ldflags -X.
var version = "dev"

func Version() string {
	if v := strings.TrimSpace(version); v != "" && v != "dev" {
		return strings.TrimPrefix(v, "v")
	}
	if info, ok := debug.ReadBuildInfo(); ok {
		if v := strings.TrimSpace(info.Main.Version); v != "" && v != "(devel)" {
			return strings.TrimPrefix(v, "v")
		}
	}
	return "dev"
}

func UserAgent() string {
	return Name + "/" + Version()
}

func WebUserAgent() string {
	return UserAgent() + " (+" + Repository + ")"
}
