package presentation

type Density uint8

const (
	DensityMinimal Density = iota
	DensityNormal
	DensityDebug
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
