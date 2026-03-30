package workload

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

// ArrivalScheduler determines the time interval between workload submissions.
type ArrivalScheduler interface {
	// NextInterval returns the duration to wait before submitting the next workload.
	NextInterval() time.Duration
}

// NewArrivalScheduler creates an ArrivalScheduler from the given config.
// rng should be the sampler's RNG so that arrival times share the same seed.
func NewArrivalScheduler(pattern config.ArrivalPattern, rng *rand.Rand) (ArrivalScheduler, error) {
	if pattern.RatePerMinute == nil {
		return nil, fmt.Errorf("arrivalPattern.ratePerMinute is required")
	}
	rate := *pattern.RatePerMinute
	if rate <= 0 {
		return nil, fmt.Errorf("arrivalPattern.ratePerMinute must be > 0, got %g", rate)
	}

	switch pattern.Type {
	case "constant":
		interval := time.Duration(float64(time.Minute) / rate)
		return &ConstantScheduler{interval: interval}, nil
	case "poisson":
		// lambda = arrivals per second
		lambda := rate / 60.0
		return &PoissonScheduler{lambda: lambda, rng: rng}, nil
	default:
		return nil, fmt.Errorf("unsupported arrival pattern type %q", pattern.Type)
	}
}

// ConstantScheduler returns a fixed interval between workload submissions.
type ConstantScheduler struct {
	interval time.Duration
}

// NextInterval returns the fixed interval.
func (c *ConstantScheduler) NextInterval() time.Duration {
	return c.interval
}

// PoissonScheduler returns exponentially-distributed inter-arrival times,
// modeling a Poisson process with the given arrival rate.
//
// If arrivals follow a Poisson process with rate λ (arrivals/second), the
// inter-arrival time T is exponentially distributed: T ~ Exp(λ).
// Sampled as T = Exp(1) / λ using rand.ExpFloat64().
type PoissonScheduler struct {
	lambda float64 // arrivals per second
	rng    *rand.Rand
}

// NextInterval returns the next exponentially-distributed inter-arrival time.
func (p *PoissonScheduler) NextInterval() time.Duration {
	secs := p.rng.ExpFloat64() / p.lambda
	return time.Duration(secs * float64(time.Second))
}
