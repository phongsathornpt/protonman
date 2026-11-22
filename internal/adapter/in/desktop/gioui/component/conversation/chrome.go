//go:build desktop || desktop_gio

package conversation

import (
	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/component/uikit"
)

// Chrome defines styling tokens and rendering delegates for conversation
// presentation. It is the shared uikit surface: conversation adds no additional
// visual primitives of its own.
type Chrome = uikit.Chrome
