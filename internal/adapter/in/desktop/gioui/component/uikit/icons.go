//go:build desktop || desktop_gio

package uikit

// Icon identifies a glyph rendered by the host's icon painter.
//
// Components request icons by name so they never carry icon path data or a
// dependency on the shell's icon table. The host maps an Icon onto its own
// drawing implementation; an unmapped icon must fall back to something neutral
// rather than failing the frame.
type Icon string

// The icon vocabulary used across the desktop frontend.
const (
	IconBrandLogo    Icon = "brand-logo"
	IconSearch       Icon = "search"
	IconClose        Icon = "close"
	IconChevronDown  Icon = "chevron-down"
	IconChevronRight Icon = "chevron-right"
	IconChevronUp    Icon = "chevron-up"
	IconKebab        Icon = "kebab"
	IconStar         Icon = "star"
	IconSettings     Icon = "settings"
	IconCompose      Icon = "compose"
	IconFolder       Icon = "folder"
	IconArchive      Icon = "archive"
	IconDiscord      Icon = "discord"
	IconTerminal     Icon = "terminal"
	IconCheck        Icon = "check"
	IconCopy         Icon = "copy"
	IconTrash        Icon = "trash"
	IconPlus         Icon = "plus"
)
