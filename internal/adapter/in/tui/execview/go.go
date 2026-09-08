package execview

import "strings"

func summarizeGoExec(p *Presentation, output string) {
	lines := nonEmptyExecLines(output)
	switch p.Action {
	case "test":
		passed, noTests, failed := 0, 0, 0
		for _, line := range lines {
			switch {
			case strings.HasPrefix(line, "ok\t") || strings.HasPrefix(line, "ok  "):
				passed++
			case strings.HasPrefix(line, "?\t") || strings.HasPrefix(line, "?  "):
				noTests++
			case strings.HasPrefix(line, "FAIL\t") || strings.HasPrefix(line, "FAIL "):
				failed++
			}
		}
		parts := []string{}
		if passed > 0 {
			parts = append(parts, pluralCount(passed, "package passed", "packages passed"))
		}
		if failed > 0 {
			parts = append(parts, pluralCount(failed, "package failed", "packages failed"))
		}
		if noTests > 0 {
			parts = append(parts, pluralCount(noTests, "no tests", "no tests"))
		}
		if len(parts) > 0 {
			p.Summary = strings.Join(parts, " · ")
			p.SuppressRaw = failed == 0
		}
	case "vet":
		if len(lines) == 0 {
			p.Summary, p.SuppressRaw = "clean", true
		}
	case "build":
		if len(lines) == 0 {
			p.SuppressRaw = true
		}
	case "fmt":
		if len(lines) > 0 {
			p.Summary = pluralCount(len(lines), "file formatted", "files formatted")
			p.Details = capExecLines(lines, 5)
			p.SuppressRaw = true
		}
	case "mod":
		p.Details = capExecLines(lines, 4)
		p.SuppressRaw = len(lines) > 0
	}
}
