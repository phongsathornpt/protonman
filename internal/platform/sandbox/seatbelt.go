package sandbox

import "strings"

func seatbeltProfile(profile Profile, dir string) string {
	var builder strings.Builder
	builder.WriteString("(version 1)\n(allow default)\n")
	builder.WriteString("(allow file-read*)\n")
	builder.WriteString("(allow process-exec)\n")
	builder.WriteString("(allow process-fork)\n")
	builder.WriteString("(allow signal)\n")
	builder.WriteString("(allow sysctl-read)\n")
	builder.WriteString("(deny file-write*\n")
	builder.WriteString("  (require-all\n")
	// Seatbelt deny rules take precedence over allow rules. A writable
	// workspace therefore has to be excluded from the deny predicate itself;
	// a later `(allow file-write* (subpath ...))` cannot override a global
	// deny. Read-only profiles intentionally omit this exclusion.
	if !profile.ReadOnly {
		builder.WriteString("    (require-not (subpath " + seatbeltString(dir) + "))\n")
	}
	// Keep the small set of standard writable character devices usable. Basic
	// shell/tool execution commonly redirects to /dev/null or reads randomness;
	// denying these is unrelated to workspace filesystem confinement.
	for _, device := range []string{"/dev/null", "/dev/zero", "/dev/random", "/dev/urandom"} {
		builder.WriteString("    (require-not (literal " + seatbeltString(device) + "))\n")
	}
	builder.WriteString("  )\n")
	builder.WriteString(")\n")
	if profile.RestrictNetwork {
		builder.WriteString("(deny network*)\n")
	}
	return builder.String()
}

func seatbeltString(value string) string {
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}
