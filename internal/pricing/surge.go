package pricing

import "ridehailing/internal/domain"

type NoSurge struct{}

func (NoSurge) Multiplier(domain.SurgeInput) domain.Multiplier { return 100 }

// SurgeStep applies while demand <= Factor * supply. Steps are checked in order and the first match wins.
type SurgeStep struct {
	Factor     int
	Multiplier domain.Multiplier
}

type StepSurge struct {
	Steps []SurgeStep
	Cap   domain.Multiplier
}

func DefaultStepSurge() StepSurge {
	return StepSurge{
		Steps: []SurgeStep{
			{Factor: 1, Multiplier: 100},
			{Factor: 2, Multiplier: 125},
			{Factor: 3, Multiplier: 150},
		},
		Cap: 200,
	}
}

// Multiplier compares demand against multiples of supply rather than dividing, so zero supply needs no special case.
func (s StepSurge) Multiplier(in domain.SurgeInput) domain.Multiplier {
	for _, step := range s.Steps {
		if in.Demand <= step.Factor*in.Supply {
			return step.Multiplier
		}
	}
	return s.Cap
}
