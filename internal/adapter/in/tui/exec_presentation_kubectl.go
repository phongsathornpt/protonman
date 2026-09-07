package tui

import "strings"

func kubectlAction(args []string) string {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if arg == "rollout" && i+1 < len(args) {
			return "rollout " + args[i+1]
		}
		return arg
	}
	return ""
}

func kubectlExecTitle(_ string, args []string, action string) string {
	if action == "get" {
		for i, arg := range args {
			if arg == "get" && i+1 < len(args) {
				return execTitle("Kubectl get", args[i+1])
			}
		}
	}
	return execTitle("Kubectl", action)
}

func summarizeKubectlExec(p *execPresentation, output string) {
	lines := nonEmptyExecLines(output)
	switch {
	case strings.HasPrefix(p.Action, "rollout"):
		if strings.Contains(strings.ToLower(output), "successfully rolled out") {
			p.Summary, p.SuppressRaw = "rollout complete", true
		}
	case p.Action == "apply":
		created, configured, unchanged := 0, 0, 0
		for _, line := range lines {
			trim := strings.TrimSpace(line)
			switch {
			case strings.HasSuffix(trim, " created"):
				created++
			case strings.HasSuffix(trim, " configured"):
				configured++
			case strings.HasSuffix(trim, " unchanged"):
				unchanged++
			}
		}
		parts := []string{}
		if configured > 0 {
			parts = append(parts, pluralCount(configured, "configured", "configured"))
		}
		if created > 0 {
			parts = append(parts, pluralCount(created, "created", "created"))
		}
		if unchanged > 0 {
			parts = append(parts, pluralCount(unchanged, "unchanged", "unchanged"))
		}
		if len(parts) > 0 {
			p.Summary, p.SuppressRaw = strings.Join(parts, " · "), true
		}
	case p.Action == "delete":
		if len(lines) > 0 {
			p.Summary = pluralCount(len(lines), "resource deleted", "resources deleted")
		}
	case p.Action == "get":
		// Keep the table visible; only add a compact count when the standard header is present.
		if len(lines) > 1 && strings.HasPrefix(strings.TrimSpace(lines[0]), "NAME") {
			p.Summary = pluralCount(len(lines)-1, "resource", "resources")
		}
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
