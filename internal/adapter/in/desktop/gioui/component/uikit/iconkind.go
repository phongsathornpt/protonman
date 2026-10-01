//go:build desktop || desktop_gio

package uikit

// IconKind is the host-side drawing variant for an Icon. Components request
// icons by name; only the host knows the vector data, so the resolution table
// lives here beside the vocabulary it resolves.
type IconKind uint8

const (
	KindSidebar IconKind = iota
	KindInspector
	KindSearch
	KindAdd
	KindSend
	KindStop
	KindClose
	KindChevronDown
	KindChevronRight
	KindChevronUp
	KindKebab
	KindStar
	KindSettings
	KindCompose
	KindFolder
	KindArchive
	KindDiscord
	KindBrandLogo
	KindTerminal
	KindCheck
	KindCopy
)

// ResolveIconKind maps a component-requested icon onto its drawing variant. An
// unmapped icon falls back to the close glyph rather than failing the frame, so
// adding a name to the vocabulary can never render an empty element.
func ResolveIconKind(icon Icon) IconKind {
	if kind, ok := iconKindByName[icon]; ok {
		return kind
	}
	return KindClose
}

var iconKindByName = map[Icon]IconKind{
	IconBrandLogo:    KindBrandLogo,
	IconSearch:       KindSearch,
	IconClose:        KindClose,
	IconChevronDown:  KindChevronDown,
	IconChevronRight: KindChevronRight,
	IconChevronUp:    KindChevronUp,
	IconKebab:        KindKebab,
	IconStar:         KindStar,
	IconSettings:     KindSettings,
	IconCompose:      KindCompose,
	IconFolder:       KindFolder,
	IconArchive:      KindArchive,
	IconDiscord:      KindDiscord,
	IconTerminal:     KindTerminal,
	IconCheck:        KindCheck,
	IconCopy:         KindCopy,
}
