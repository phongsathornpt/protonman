package analysis

import "math"

// Summary is a bounded-memory numeric aggregate.
type Summary struct {
	Count  int     `json:"count"`
	Min    float64 `json:"min"`
	Max    float64 `json:"max"`
	Mean   float64 `json:"mean"`
	StdDev float64 `json:"stddev"`
}

// RunningStats incrementally computes stable numeric aggregates.
type RunningStats struct {
	count int
	min   float64
	max   float64
	mean  float64
	m2    float64
}

func (s *RunningStats) Add(value float64) {
	s.count++
	if s.count == 1 {
		s.min, s.max, s.mean = value, value, value
		return
	}
	if value < s.min {
		s.min = value
	}
	if value > s.max {
		s.max = value
	}
	delta := value - s.mean
	s.mean += delta / float64(s.count)
	s.m2 += delta * (value - s.mean)
}

func (s RunningStats) Summary() Summary {
	if s.count == 0 {
		return Summary{}
	}
	stddev := 0.0
	if s.count > 1 {
		stddev = math.Sqrt(s.m2 / float64(s.count-1))
	}
	return Summary{
		Count: s.count, Min: s.min, Max: s.max,
		Mean: s.mean, StdDev: stddev,
	}
}
