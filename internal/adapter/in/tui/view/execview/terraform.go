package execview

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	terraformPlanRE  = regexp.MustCompile(`Plan:\s*(\d+) to add,\s*(\d+) to change,\s*(\d+) to destroy`)
	terraformApplyRE = regexp.MustCompile(`Resources:\s*(\d+) added,\s*(\d+) changed,\s*(\d+) destroyed`)
)

func terraformExecTitle(name string, _ []string, action string) string {
	label := "Terraform"
	if name == "tofu" {
		label = "OpenTofu"
	}
	return execTitle(label, action)
}

func summarizeTerraformExec(p *Presentation, output string) {
	switch p.Action {
	case "plan":
		if m := terraformPlanRE.FindStringSubmatch(output); len(m) == 4 {
			added, okAdded := atoiExec(m[1])
			changed, okChanged := atoiExec(m[2])
			destroyed, okDestroyed := atoiExec(m[3])
			if okAdded && okChanged && okDestroyed {
				p.Summary = fmt.Sprintf("+%d ~%d -%d", added, changed, destroyed)
				p.SuppressRaw = true
			}
		} else if strings.Contains(output, "No changes.") {
			p.Summary, p.SuppressRaw = "no changes", true
		}
	case "apply":
		if m := terraformApplyRE.FindStringSubmatch(output); len(m) == 4 {
			added, okAdded := atoiExec(m[1])
			changed, okChanged := atoiExec(m[2])
			destroyed, okDestroyed := atoiExec(m[3])
			if okAdded && okChanged && okDestroyed {
				p.Summary = strings.Join([]string{
					pluralCount(added, "added", "added"),
					pluralCount(changed, "changed", "changed"),
					pluralCount(destroyed, "destroyed", "destroyed"),
				}, " · ")
				p.SuppressRaw = true
			}
		}
	case "validate":
		if strings.Contains(strings.ToLower(output), "configuration is valid") {
			p.Summary, p.SuppressRaw = "valid configuration", true
		}
	case "fmt":
		lines := nonEmptyExecLines(output)
		if len(lines) == 0 {
			p.SuccessSummary, p.SuppressRaw = "clean", true
		} else {
			p.Summary = pluralCount(len(lines), "file formatted", "files formatted")
			p.Details = capExecLines(lines, 5)
			p.SuppressRaw = true
		}
	}
	attachFailureDetails(p, output)
}
