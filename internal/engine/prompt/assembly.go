package prompt

import (
	"sort"
	"strings"
)

// Section is one deterministic contribution to the model-facing system prompt.
// Orders are intentionally sparse so stable and semi-stable sections stay in the
// reusable prefix while turn/session volatility is pushed toward the suffix.
type Section struct {
	Name  string
	Order int
	Text  string
}

const (
	orderIdentity = -1000

	orderExecution        = 0
	orderToolDiscipline   = 100
	orderWorkspace        = 200
	orderTaskCoordination = 300
	orderDelegation       = 400
	orderMCP              = 500
	orderVerification     = 600

	orderProjectInstructions    = 3000
	orderAdditionalInstructions = 4000

	orderRole       = 7000
	orderActiveGoal = 8000
	orderSkills     = 9000
	orderGrounding  = 10000
)

func renderSections(sections []Section) string {
	filtered := make([]Section, 0, len(sections))
	for _, section := range sections {
		section.Name = strings.TrimSpace(section.Name)
		section.Text = strings.TrimSpace(section.Text)
		if section.Name == "" || section.Text == "" {
			continue
		}
		filtered = append(filtered, section)
	}

	sort.Slice(filtered, func(i, j int) bool {
		if filtered[i].Order != filtered[j].Order {
			return filtered[i].Order < filtered[j].Order
		}
		return filtered[i].Name < filtered[j].Name
	})

	parts := make([]string, 0, len(filtered))
	for _, section := range filtered {
		parts = append(parts, section.Text)
	}
	return strings.Join(parts, "\n\n")
}
