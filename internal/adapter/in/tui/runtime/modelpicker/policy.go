package modelpicker

import (
	"fmt"
	"sort"
	"strings"

	"github.com/phongsathornpt/protonman/internal/adapter/out/config"
	"github.com/phongsathornpt/protonman/internal/adapter/out/model"
)

func ProviderNames(providers map[string]config.ProviderConfig, active string) ([]string, int) {
	names := make([]string, 0, len(providers)+1)
	seen := make(map[string]bool, len(providers)+1)
	for name := range providers {
		names = append(names, name)
		seen[strings.ToLower(name)] = true
	}
	sort.Strings(names)
	if active != "" && !seen[strings.ToLower(active)] {
		names = append([]string{active}, names...)
	}
	if active == "" && len(names) == 0 {
		names = append(names, model.DefaultProtonmanName)
	}
	index := 0
	for i, name := range names {
		if strings.EqualFold(name, active) {
			index = i
			break
		}
	}
	return names, index
}

func FormatTokens(tokens int) string {
	if tokens >= 1000000 {
		return fmt.Sprintf("%.1fM", float64(tokens)/1000000.0)
	}
	if tokens >= 1000 {
		return fmt.Sprintf("%dK", tokens/1000)
	}
	return fmt.Sprintf("%d", tokens)
}

func FormatTokenLimits(contextWindow, maxInput, maxOutput int) string {
	parts := make([]string, 0, 3)
	if contextWindow > 0 {
		parts = append(parts, FormatTokens(contextWindow)+" context")
	}
	if maxInput > 0 {
		parts = append(parts, FormatTokens(maxInput)+" input")
	}
	if maxOutput > 0 {
		parts = append(parts, FormatTokens(maxOutput)+" output")
	}
	return strings.Join(parts, " · ")
}
