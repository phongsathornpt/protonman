package provider

import "strings"

func FilterValue(entry SelectionEntry) string {
	return strings.Join([]string{entry.Name, entry.DisplayName, entry.BaseURL, entry.Description}, " ")
}

func Title(entry SelectionEntry) string {
	status := "setup"
	switch {
	case entry.IsActive:
		status = "active"
	case entry.IsConfigured:
		status = "saved"
	case entry.Kind == SelectionCustom:
		status = "custom"
	}
	label := entry.DisplayName + " · " + status
	if entry.IsFree {
		label += " · free"
	}
	if entry.IsActive {
		label = "✓ " + label
	}
	return label
}
func Description(entry SelectionEntry) string {
	if strings.TrimSpace(entry.BaseURL) != "" {
		return entry.BaseURL
	}
	return entry.Description
}

func Marker(entry SelectionEntry) string {
	switch {
	case entry.IsActive:
		return "(current)"
	case entry.IsConfigured:
		return "saved"
	default:
		return ""
	}
}
