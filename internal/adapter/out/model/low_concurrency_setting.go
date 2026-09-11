package model

import (
	"fmt"
	"strings"
)

// LowConcurrencySetting controls whether the low-concurrency wrapper is applied.
// Auto preserves provider policy: OpenCode free models use it, other routes do not.
type LowConcurrencySetting uint8

const (
	LowConcurrencyAuto LowConcurrencySetting = iota
	LowConcurrencyOn
	LowConcurrencyOff
)

func (s LowConcurrencySetting) String() string {
	switch s {
	case LowConcurrencyOn:
		return "on"
	case LowConcurrencyOff:
		return "off"
	default:
		return "auto"
	}
}

func ParseLowConcurrencySetting(raw string) (LowConcurrencySetting, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "auto":
		return LowConcurrencyAuto, nil
	case "on", "enable", "enabled":
		return LowConcurrencyOn, nil
	case "off", "disable", "disabled":
		return LowConcurrencyOff, nil
	default:
		return LowConcurrencyAuto, fmt.Errorf("invalid low concurrency mode %q; expected auto, on, or off", raw)
	}
}

func (s LowConcurrencySetting) Enabled(isOpenCode, freeModel bool) bool {
	if !isOpenCode {
		return false
	}
	switch s {
	case LowConcurrencyOn:
		return true
	case LowConcurrencyOff:
		return false
	default:
		return freeModel
	}
}
