package pane

import (
	"fmt"
	"strings"

	tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"
	"github.com/phongsathornpt/protonman/internal/core/tool"
)

type ProjectFact struct {
	Label  string
	Value  string
	Source string
}

type ProjectSnapshot struct {
	Width        int
	Height       int
	WorkDir      string
	RootName     string
	ConfigName   string
	TrustEnv     string
	Loading      bool
	ErrorText    string
	Exists       bool
	ConfigExists bool
	ConfigLoaded bool
	Trusted      bool
	SkillsExists bool
	SkillCount   int
	Facts        []ProjectFact
	Notice       string
}

func ProjectRows(snapshot ProjectSnapshot) []string {
	rows := []string{tuistyle.BrandStyle.Render("Project Settings")}
	root := strings.TrimSpace(snapshot.WorkDir)
	if root == "" {
		root = "."
	}
	rows = append(rows, tuistyle.MutedStyle.Render(tool.TruncateRunes(root, max(12, snapshot.Width-10))), "")
	if snapshot.Loading {
		rows = append(rows, tuistyle.MutedStyle.Render("Loading "+snapshot.RootName+" workspace state..."), "", tuistyle.MutedStyle.Render("esc close"))
		return rows
	}
	if snapshot.ErrorText != "" {
		rows = append(rows,
			tuistyle.ErrorStyle.Render("Failed to inspect "+snapshot.RootName),
			tuistyle.MutedStyle.Render(tool.TruncateRunes(snapshot.ErrorText, max(12, snapshot.Width-10))),
			"",
			tuistyle.MutedStyle.Render("r retry · esc close"),
		)
		return rows
	}
	rootStatus := "not found"
	if snapshot.Exists {
		rootStatus = "detected"
	}
	rows = append(rows, ProjectFactLine(snapshot.RootName, rootStatus))
	configStatus := "not found"
	switch {
	case snapshot.ConfigLoaded:
		configStatus = "loaded · trusted"
	case snapshot.ConfigExists && !snapshot.Trusted:
		configStatus = "ignored · untrusted"
	case snapshot.ConfigExists && snapshot.Trusted:
		configStatus = "detected · restart for full reload"
	case snapshot.ConfigExists:
		configStatus = "detected"
	}
	rows = append(rows, ProjectFactLine("Config", configStatus))
	skillsStatus := "not found"
	if snapshot.SkillsExists {
		skillsStatus = fmt.Sprintf("%d detected", snapshot.SkillCount)
		if !snapshot.Trusted && snapshot.SkillCount > 0 {
			skillsStatus += " · inactive until trusted"
		}
	}
	rows = append(rows, ProjectFactLine("Skills", skillsStatus), "")
	for _, fact := range snapshot.Facts {
		value := fact.Value
		if fact.Source != "" {
			value += " · " + fact.Source
		}
		rows = append(rows, ProjectFactLine(fact.Label, value))
	}
	rows = append(rows, "")
	if snapshot.Notice != "" {
		rows = append(rows, tuistyle.SuccessStyle.Render(snapshot.Notice), "")
	}
	if snapshot.ConfigExists && !snapshot.Trusted {
		rows = append(rows,
			tuistyle.WarningStyle.Render("Project config and skills are present but not trusted."),
			tuistyle.MutedStyle.Render("Restart with "+snapshot.TrustEnv+"=1 to enable project-local settings."),
		)
	} else if !snapshot.Exists {
		rows = append(rows, tuistyle.MutedStyle.Render("No project-local Protonman settings are configured."))
	}
	rows = append(rows, tuistyle.MutedStyle.Render("/project set <setting> <value> · r reload · esc close"))
	if ModeForHeight(snapshot.Height) == LayoutTiny {
		rows = CompactRows(rows)
	}
	return rows
}

func ProjectFactLine(label, value string) string { return fmt.Sprintf("%-12s %s", label, value) }

func FallbackValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func FormatLimit(value int) string {
	if value <= 0 {
		return "unbounded"
	}
	return fmt.Sprintf("%d", value)
}
