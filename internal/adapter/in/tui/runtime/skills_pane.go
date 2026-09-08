package runtime

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/phongsathornpt/protonman/internal/adapter/in/tui/view/pane"
	"github.com/phongsathornpt/protonman/internal/app/appdirs"
)

const skillsViewID = "skills"
const maxSkillsRows = 6

type skillsPaneView struct {
	index  int
	offset int
}

func (*skillsPaneView) ID() string             { return skillsViewID }
func (*skillsPaneView) ReplacesComposer() bool { return true }

func (v *skillsPaneView) Render(m *bubbleModel) string {
	items := make([]pane.SkillItem, 0)
	if m != nil && m.skills != nil {
		skills := m.skills.List()
		items = make([]pane.SkillItem, 0, len(skills))
		for _, skill := range skills {
			items = append(items, pane.SkillItem{Name: skill.Name, Active: m.skills.IsActivated(skill.Name)})
		}
	}
	rows := pane.SkillsRows(pane.SkillsSnapshot{
		Width:             m.width,
		Height:            m.height,
		Index:             v.index,
		Offset:            v.offset,
		Items:             items,
		UserSkillsDisplay: appdirs.UserSkillsDisplay(),
	})
	return renderModalRows(m, accentAssistant, rows)
}

func (v *skillsPaneView) HandleKey(m *bubbleModel, message tea.KeyMsg) (bool, tea.Cmd) {
	defer func() {
		if m != nil && m.skills != nil {
			visible := pickerVisibleRows(m.height, maxSkillsRows)
			v.index, v.offset, _ = normalizedPickerWindow(v.index, v.offset, len(m.skills.List()), visible)
		}
	}()
	if key.Matches(message, m.keys.ToggleSkills) {
		m.bottom.remove(skillsViewID)
		return true, nil
	}
	skills := m.skills.List()
	if len(skills) == 0 {
		m.bottom.remove(skillsViewID)
		return true, nil
	}
	switch message.String() {
	case "up", "k":
		v.index--
		if v.index < 0 {
			v.index = len(skills) - 1
		}
		return true, nil
	case "down", "j":
		v.index = (v.index + 1) % len(skills)
		return true, nil
	case "pgup":
		v.index = max(0, v.index-5)
		return true, nil
	case "pgdown":
		v.index = min(len(skills)-1, v.index+5)
		return true, nil
	case "home", "g":
		v.index = 0
		return true, nil
	case "end", "G":
		v.index = len(skills) - 1
		return true, nil
	case " ", "t":
		if v.index >= 0 && v.index < len(skills) {
			_, _ = m.skills.Toggle(skills[v.index].Name)
		}
		return true, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		num := int(message.Runes[0] - '1')
		if num >= 0 && num < len(skills) {
			v.index = num
		}
		return true, nil
	case "esc", "enter", "q":
		m.bottom.remove(skillsViewID)
		return true, nil
	default:
		return false, nil
	}
}
