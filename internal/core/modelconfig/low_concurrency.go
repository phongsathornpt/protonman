package modelconfig

import (
	"fmt"
	"strings"
)

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

func (s LowConcurrencySetting) Enabled(autoRecommended bool) bool {
	switch s {
	case LowConcurrencyOn:
		return true
	case LowConcurrencyOff:
		return false
	default:
		return autoRecommended
	}
}
