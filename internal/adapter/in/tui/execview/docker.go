package execview

import "strings"

func dockerAction(args []string) string {
	if len(args) == 0 {
		return ""
	}
	if args[0] == "compose" && len(args) > 1 {
		return "compose " + args[1]
	}
	return args[0]
}

func dockerExecTitle(name string, _ []string, action string) string {
	if name == "docker-compose" {
		return execTitle("Docker compose", action)
	}
	return execTitle("Docker", action)
}

func summarizeDockerExec(p *Presentation, output string) {
	lines := nonEmptyExecLines(output)
	switch p.Action {
	case "build", "compose build":
		if hasExecLine(output, "successfully built") || hasExecLine(output, "exporting to image") {
			p.Summary = "built image"
			p.SuppressRaw = true
		}
	case "compose up":
		count := 0
		for _, line := range lines {
			lower := strings.ToLower(line)
			if strings.Contains(lower, "running") || strings.Contains(lower, "started") {
				count++
			}
		}
		if count > 0 {
			p.Summary = pluralCount(count, "service running", "services running")
		}
	case "pull":
		if hasExecLine(output, "downloaded newer image") || hasExecLine(output, "image is up to date") {
			p.Summary = "image ready"
			p.SuppressRaw = true
		}
	}
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}

func hasExecLine(output, fragment string) bool {
	return strings.Contains(strings.ToLower(output), strings.ToLower(fragment))
}
