package presentation

import "github.com/phongsathornpt/protonman/internal/core/tool"

type Density uint8

const (
	DensityMinimal Density = iota
	DensityNormal
	DensityDebug
)

type DetailLevel uint8

const (
	DetailSummary DetailLevel = iota
	DetailMaterial
	DetailDiagnostic
)

type Policy struct {
	Density Density
}

func MinimalPolicy() Policy {
	return Policy{Density: DensityMinimal}
}

func (p Policy) ShowRoutineDetail() bool {
	return p.Density != DensityMinimal
}

func (p Policy) ShowDebugDetail() bool {
	return p.Density == DensityDebug
}

func (p Policy) ToolDetail(kind tool.Kind, denied bool, failed bool) DetailLevel {
	if denied || failed {
		return DetailDiagnostic
	}
	if p.Density == DensityDebug {
		return DetailDiagnostic
	}
	if p.Density == DensityNormal {
		return DetailMaterial
	}
	switch kind {
	case tool.KindEdit, tool.KindGit, tool.KindBash:
		return DetailMaterial
	default:
		return DetailSummary
	}
}
