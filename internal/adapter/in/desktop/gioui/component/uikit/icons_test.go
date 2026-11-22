//go:build desktop || desktop_gio

package uikit

import "testing"

func TestIconVocabularyResolves(t *testing.T) {
	icons := []Icon{
		IconSidebar, IconInspector, IconSearch, IconClose,
		IconChevronDown, IconChevronRight, IconChevronUp,
		IconKebab, IconStar, IconSettings, IconCompose,
		IconFolder, IconArchive, IconDiscord, IconTerminal,
		IconCheck, IconCopy, IconTrash, IconPlus,
		IconBrandLogo, IconSend, IconStop,
	}
	seen := make(map[IconKind]Icon, len(icons))
	for _, icon := range icons {
		if icon == "" {
			t.Fatalf("empty icon name in vocabulary")
		}
		kind, ok := iconKindByName[icon]
		if !ok {
			t.Fatalf("icon %q missing from iconKindByName", icon)
		}
		if prev, dup := seen[kind]; dup {
			t.Fatalf("icons %q and %q share kind %d", prev, icon, kind)
		}
		seen[kind] = icon
		if got := ResolveIconKind(icon); got != kind {
			t.Fatalf("ResolveIconKind(%q) = %d, want %d", icon, got, kind)
		}
	}
	if got := ResolveIconKind(Icon("nope-missing")); got != KindClose {
		t.Fatalf("unknown icon fallback = %d, want KindClose", got)
	}
}
