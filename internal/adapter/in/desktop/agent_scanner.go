//go:build desktop

package desktop

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// DiscoveredAgent represents an ACP-compatible CLI found on the local machine.
type DiscoveredAgent struct {
	ID          string   `json:"id"`
	DisplayName string   `json:"displayName"`
	Command     string   `json:"command"`
	Args        []string `json:"args"`
	Description string   `json:"description"`
}

// OptionLabel formats the discovered agent for display in dropdown menus.
func (d DiscoveredAgent) OptionLabel() string {
	cmdStr := filepath.Base(d.Command)
	if len(d.Args) > 0 {
		cmdStr += " " + strings.Join(d.Args, " ")
	}
	return fmt.Sprintf("%s (%s)", d.DisplayName, cmdStr)
}

// LookPathFunc abstracts exec.LookPath for testing.
type LookPathFunc func(string) (string, error)

// GetenvFunc abstracts os.Getenv for testing.
type GetenvFunc func(string) string

// ScanMachineACPAgents scans the host machine for CLIs with ACP support.
func ScanMachineACPAgents() []DiscoveredAgent {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return scanACPAgentsWith(ctx, exec.LookPath, os.Getenv)
}

func scanACPAgentsWith(ctx context.Context, lookPath LookPathFunc, getenv GetenvFunc) []DiscoveredAgent {
	var discovered []DiscoveredAgent
	seen := make(map[string]bool)

	add := func(agent DiscoveredAgent) {
		agent.ID = strings.ToLower(strings.TrimSpace(agent.ID))
		if agent.ID == "" || agent.Command == "" {
			return
		}
		if seen[agent.ID] {
			return
		}
		seen[agent.ID] = true
		discovered = append(discovered, agent)
	}

	// 1. Check Protonman binary
	protonPath := getenv("PROTONMAN_BINARY")
	if protonPath == "" {
		if path, err := lookPath("protonman"); err == nil {
			protonPath = path
		} else if exe, err := os.Executable(); err == nil {
			if isProtonmanExecutable(exe) {
				protonPath = exe
			} else {
				sibling := filepath.Join(filepath.Dir(exe), "protonman")
				if fileExists(sibling) {
					protonPath = sibling
				}
			}
		}
	}
	if protonPath != "" {
		add(DiscoveredAgent{
			ID:          defaultAgentID,
			DisplayName: "Protonman",
			Command:     protonPath,
			Args:        []string{"--acp"},
			Description: "Protonman autonomous coding agent (ACP stdio)",
		})
	}

	// 2. Check Google Antigravity
	agyPath := getenv(antigravityCommandEnv)
	if agyPath == "" {
		if path, err := lookPath("agy_acp_server.par"); err == nil {
			agyPath = path
		} else if path, err := lookPath("antigravity"); err == nil {
			agyPath = path
		}
	}
	if agyPath == "" {
		home := getenv("HOME")
		candidates := []string{
			"/opt/agy/agy_acp_server.par",
			filepath.Join(home, ".gemini/antigravity-cli/bin/agy_acp_server.par"),
			"/Applications/Antigravity.app/Contents/Resources/app/bin/agy_acp_server.par",
		}
		for _, cand := range candidates {
			if fileExists(cand) {
				agyPath = cand
				break
			}
		}
	}
	if agyPath != "" {
		args, _ := parseAgentArguments(getenv(antigravityArgumentsEnv))
		add(DiscoveredAgent{
			ID:          antigravityAgentID,
			DisplayName: "Google Antigravity",
			Command:     agyPath,
			Args:        args,
			Description: "Google Antigravity ACP server",
		})
	}

	// 3. Check well-known ACP-supporting CLIs (e.g. goose, zero)
	knownProbes := []struct {
		id          string
		name        string
		defaultArgs []string
		desc        string
	}{
		{id: "goose", name: "Block Goose", defaultArgs: []string{"acp"}, desc: "Goose open-source AI agent (acp stdio)"},
		{id: "zero", name: "Zero", defaultArgs: []string{"acp"}, desc: "Zero Agent Client Protocol server"},
		{id: "claude", name: "Claude Code", defaultArgs: []string{"--acp"}, desc: "Anthropic Claude Code ACP agent"},
		{id: "claude-code", name: "Claude Code", defaultArgs: []string{"--acp"}, desc: "Anthropic Claude Code ACP agent"},
	}

	for _, probe := range knownProbes {
		if path, err := lookPath(probe.id); err == nil {
			if ok, args, helpDesc := probeACPCLI(ctx, path); ok {
				if len(args) == 0 {
					args = probe.defaultArgs
				}
				desc := probe.desc
				if helpDesc != "" {
					desc = helpDesc
				}
				add(DiscoveredAgent{
					ID:          probe.id,
					DisplayName: probe.name,
					Command:     path,
					Args:        args,
					Description: desc,
				})
			}
		}
	}

	// 4. Scan PATH directories for binaries matching ACP conventions
	pathEnv := getenv("PATH")
	probedCount := 0
	const maxPathProbes = 16
	for _, dir := range filepath.SplitList(pathEnv) {
		if ctx.Err() != nil || probedCount >= maxPathProbes {
			break
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if ctx.Err() != nil || probedCount >= maxPathProbes {
				break
			}
			if entry.IsDir() {
				continue
			}
			name := entry.Name()
			if !isLikelyACPBinaryName(name) {
				continue
			}
			fullPath := filepath.Join(dir, name)
			id := sanitizeAgentID(name)
			if seen[id] {
				continue
			}
			probedCount++
			if ok, args, desc := probeACPCLI(ctx, fullPath); ok {
				add(DiscoveredAgent{
					ID:          id,
					DisplayName: formatDisplayName(name),
					Command:     fullPath,
					Args:        args,
					Description: desc,
				})
			}
		}
	}

	slices.SortFunc(discovered, func(a, b DiscoveredAgent) int {
		return strings.Compare(a.DisplayName, b.DisplayName)
	})
	return discovered
}

func probeACPCLI(ctx context.Context, execPath string) (bool, []string, string) {
	probeCtx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(probeCtx, execPath, "--help")
	out, err := cmd.CombinedOutput()
	if err != nil && len(out) == 0 {
		return false, nil, ""
	}
	text := string(out)
	lower := strings.ToLower(text)

	// Check for ACP indicators
	isACP := strings.Contains(lower, "agent client protocol") ||
		strings.Contains(lower, "acp agent") ||
		strings.Contains(lower, "acp server") ||
		strings.Contains(lower, "--acp") ||
		strings.Contains(lower, " acp ") ||
		strings.Contains(lower, "\nacp") ||
		strings.Contains(lower, "\tacp")

	if !isACP {
		return false, nil, ""
	}

	var args []string
	if strings.Contains(lower, "--acp") {
		args = []string{"--acp"}
	} else if strings.Contains(lower, " acp ") || strings.Contains(lower, "\nacp") || strings.Contains(lower, "\tacp") {
		args = []string{"acp"}
	}

	desc := ""
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(strings.ToLower(line), "acp") {
			desc = line
			break
		}
	}

	return true, args, desc
}

func isLikelyACPBinaryName(name string) bool {
	nameLower := strings.ToLower(name)
	nameLower = strings.TrimSuffix(nameLower, ".exe")
	nameLower = strings.TrimSuffix(nameLower, ".par")
	if nameLower == "acp" || nameLower == "acp-server" || nameLower == "acp_server" {
		return true
	}
	if strings.HasPrefix(nameLower, "acp-") || strings.HasPrefix(nameLower, "acp_") ||
		strings.HasSuffix(nameLower, "-acp") || strings.HasSuffix(nameLower, "_acp") {
		return true
	}
	if strings.Contains(nameLower, "-acp-") || strings.Contains(nameLower, "_acp_") ||
		strings.Contains(nameLower, "-acp_") || strings.Contains(nameLower, "_acp-") {
		return true
	}
	return false
}

func isProtonmanExecutable(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	base = strings.TrimSuffix(base, ".exe")
	return base == "protonman"
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func sanitizeAgentID(name string) string {
	name = strings.TrimSuffix(name, ".par")
	name = strings.TrimSuffix(name, ".exe")
	name = strings.ToLower(name)
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func formatDisplayName(name string) string {
	name = strings.TrimSuffix(name, ".par")
	name = strings.TrimSuffix(name, ".exe")
	parts := strings.FieldsFunc(name, func(r rune) bool {
		return r == '-' || r == '_'
	})
	for i, part := range parts {
		if len(part) > 0 {
			parts[i] = strings.ToUpper(part[:1]) + part[1:]
		}
	}
	return strings.Join(parts, " ")
}
