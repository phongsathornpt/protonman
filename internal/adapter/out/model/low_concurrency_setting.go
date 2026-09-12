package model

import "github.com/phongsathornpt/protonman/internal/core/modelconfig"

type LowConcurrencySetting = modelconfig.LowConcurrencySetting

const (
	LowConcurrencyAuto = modelconfig.LowConcurrencyAuto
	LowConcurrencyOn   = modelconfig.LowConcurrencyOn
	LowConcurrencyOff  = modelconfig.LowConcurrencyOff
)

func ParseLowConcurrencySetting(raw string) (LowConcurrencySetting, error) {
	return modelconfig.ParseLowConcurrencySetting(raw)
}
