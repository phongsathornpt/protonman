package modelpicker

import (
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func DisplayName(md model.RemoteModel) string {
	if name := strings.TrimSpace(md.Name); name != "" {
		return name
	}
	id := strings.TrimSpace(md.ID)
	if model.IsFreeModel(id) {
		id = strings.TrimSuffix(strings.TrimSuffix(id, "-free"), "_free")
	}
	parts := strings.FieldsFunc(id, func(r rune) bool { return r == '-' || r == '_' })
	for index, part := range parts {
		runes := []rune(part)
		if len(runes) == 0 {
			continue
		}
		runes[0] = []rune(strings.ToUpper(string(runes[0])))[0]
		parts[index] = string(runes)
	}
	if label := strings.Join(parts, " "); label != "" {
		return label
	}
	return md.ID
}
func ActiveModelIndex(models []model.RemoteModel, activeModel string) int {
	for index, candidate := range models {
		if strings.EqualFold(candidate.ID, activeModel) {
			return index
		}
	}
	return 0
}
