package tui

import tuistyle "github.com/phongsathornpt/protonman/internal/adapter/in/tui/style"

func brandLockup(width int) string   { return tuistyle.BrandLockup(width) }
func brandLockupWidth(width int) int { return tuistyle.BrandLockupWidth(width) }
