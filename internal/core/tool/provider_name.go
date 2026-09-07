package tool

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

const maxProviderToolNameBytes = 64

// ProviderSafeName converts a canonical dynamic tool name into the conservative
// function-name grammar accepted across model providers. Changed names carry a
// stable hash suffix so sanitized collisions remain distinct.
func ProviderSafeName(name string) string {
	if providerToolNameSafe(name) {
		return name
	}
	var builder strings.Builder
	builder.Grow(len(name))
	lastUnderscore := false
	for _, r := range name {
		valid := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-'
		if valid {
			builder.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			builder.WriteByte('_')
			lastUnderscore = true
		}
	}
	base := strings.Trim(builder.String(), "_")
	if base == "" {
		base = "tool"
	}
	sum := sha256.Sum256([]byte(name))
	suffix := "_" + hex.EncodeToString(sum[:4])
	maxBase := maxProviderToolNameBytes - len(suffix)
	if len(base) > maxBase {
		base = base[:maxBase]
	}
	return base + suffix
}

func providerToolNameSafe(name string) bool {
	if name == "" || len(name) > maxProviderToolNameBytes {
		return false
	}
	for _, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-') {
			return false
		}
	}
	return true
}
