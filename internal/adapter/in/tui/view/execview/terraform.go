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
			p.Summary = fmt.Sprintf("+%d ~%d -%d", atoiExec(m[1]), atoiExec(m[2]), atoiExec(m[3]))
			p.SuppressRaw = true
		} else if strings.Contains(output, "No changes.") {
			p.Summary, p.SuppressRaw = "no changes", true
		}
	case "apply":
		if m := terraformApplyRE.FindStringSubmatch(output); len(m) == 4 {
			p.Summary = fmt.Sprintf("%d added · %d changed · %d destroyed", atoiExec(m[1]), atoiExec(m[2]), atoiExec(m[3]))
			p.SuppressRaw = true
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
	if failures := firstFailureLines(output, 3); len(failures) > 0 {
		p.Details = failures
	}
}
