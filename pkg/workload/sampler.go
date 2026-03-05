// Package workload implements workload generation for kueue-bench.
// It samples values from distributions, generates arrival schedules, and
// builds Kubernetes workload objects (Job, JobSet, RayJob) from WorkloadProfile configs.
package workload

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

// Sampler samples values from config.Distribution using a seeded random number generator.
// Three value domains are supported, matching the workload profile schema:
//   - SampleInt:      integer counts (replicas, parallelism, workerReplicas)
//   - SampleDuration: time durations (job duration annotation)
//   - SampleQuantity: resource quantities (cpu, memory, nvidia.com/gpu)
//
// The four supported distribution types (uniform, normal, lognormal, choice) cover the
// distributions defined in the WorkloadProfile schema and are implemented using Go's
// stdlib math/rand. If additional distribution types are needed (e.g. Weibull, Pareto,
// or gamma for more realistic job duration modeling), consider migrating to
// gonum.org/v1/gonum/stat/distuv, which is the standard scientific computing library
// for Go and provides a broader set of well-tested distributions.
type Sampler struct {
	rng *rand.Rand
}

// NewSampler creates a Sampler with the given seed. If seed is nil, uses time.Now().UnixNano().
func NewSampler(seed *int64) *Sampler {
	var s int64
	if seed != nil {
		s = *seed
	} else {
		s = time.Now().UnixNano()
	}
	return &Sampler{rng: rand.New(rand.NewSource(s))}
}

// SampleInt samples an integer value from the distribution.
// Fixed values and distribution parameters are parsed as base-10 integers.
// Used for: replica counts, parallelism, completions, workerReplicas.
func (s *Sampler) SampleInt(d *config.Distribution) (int64, error) {
	if d.IsFixed() {
		n, err := strconv.ParseInt(d.Value, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("fixed int value %q: %w", d.Value, err)
		}
		return n, nil
	}

	switch d.Type {
	case "uniform":
		min, err := strconv.ParseInt(d.Min, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("uniform min %q: %w", d.Min, err)
		}
		max, err := strconv.ParseInt(d.Max, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("uniform max %q: %w", d.Max, err)
		}
		if max < min {
			return 0, fmt.Errorf("uniform max (%d) < min (%d)", max, min)
		}
		return min + s.rng.Int63n(max-min+1), nil

	case "normal":
		mean, err := strconv.ParseFloat(d.Mean, 64)
		if err != nil {
			return 0, fmt.Errorf("normal mean %q: %w", d.Mean, err)
		}
		stddev, err := strconv.ParseFloat(d.Stddev, 64)
		if err != nil {
			return 0, fmt.Errorf("normal stddev %q: %w", d.Stddev, err)
		}
		sample := mean + stddev*s.rng.NormFloat64()
		return int64(math.Round(sample)), nil

	case "lognormal":
		mean, err := strconv.ParseFloat(d.Mean, 64)
		if err != nil {
			return 0, fmt.Errorf("lognormal mean %q: %w", d.Mean, err)
		}
		stddev, err := strconv.ParseFloat(d.Stddev, 64)
		if err != nil {
			return 0, fmt.Errorf("lognormal stddev %q: %w", d.Stddev, err)
		}
		mu, sigma := lognormalParams(mean, stddev)
		sample := math.Exp(mu + sigma*s.rng.NormFloat64())
		return int64(math.Round(sample)), nil

	case "choice":
		val, err := s.weightedChoice(d.Values, d.Weights)
		if err != nil {
			return 0, err
		}
		return strconv.ParseInt(val, 10, 64)

	default:
		return 0, fmt.Errorf("unsupported distribution type %q", d.Type)
	}
}

// SampleDuration samples a time.Duration from the distribution.
// Fixed values and distribution parameters are parsed as Go duration strings (e.g. "30m", "2h").
// Used for: job duration (kwok.x-k8s.io/duration annotation).
func (s *Sampler) SampleDuration(d *config.Distribution) (time.Duration, error) {
	if d.IsFixed() {
		return time.ParseDuration(d.Value)
	}

	switch d.Type {
	case "uniform":
		min, err := time.ParseDuration(d.Min)
		if err != nil {
			return 0, fmt.Errorf("uniform min %q: %w", d.Min, err)
		}
		max, err := time.ParseDuration(d.Max)
		if err != nil {
			return 0, fmt.Errorf("uniform max %q: %w", d.Max, err)
		}
		if max < min {
			return 0, fmt.Errorf("uniform max (%v) < min (%v)", max, min)
		}
		spread := max - min
		return min + time.Duration(s.rng.Int63n(int64(spread)+1)), nil

	case "normal":
		mean, err := time.ParseDuration(d.Mean)
		if err != nil {
			return 0, fmt.Errorf("normal mean %q: %w", d.Mean, err)
		}
		stddev, err := time.ParseDuration(d.Stddev)
		if err != nil {
			return 0, fmt.Errorf("normal stddev %q: %w", d.Stddev, err)
		}
		sample := float64(mean) + float64(stddev)*s.rng.NormFloat64()
		if sample < 0 {
			sample = 0
		}
		return time.Duration(math.Round(sample)), nil

	case "lognormal":
		mean, err := time.ParseDuration(d.Mean)
		if err != nil {
			return 0, fmt.Errorf("lognormal mean %q: %w", d.Mean, err)
		}
		stddev, err := time.ParseDuration(d.Stddev)
		if err != nil {
			return 0, fmt.Errorf("lognormal stddev %q: %w", d.Stddev, err)
		}
		mu, sigma := lognormalParams(float64(mean), float64(stddev))
		sample := math.Exp(mu + sigma*s.rng.NormFloat64())
		return time.Duration(math.Round(sample)), nil

	case "choice":
		val, err := s.weightedChoice(d.Values, d.Weights)
		if err != nil {
			return 0, err
		}
		return time.ParseDuration(val)

	default:
		return 0, fmt.Errorf("unsupported distribution type %q", d.Type)
	}
}

// SampleQuantity samples a resource.Quantity from the distribution.
// Fixed values and distribution parameters are parsed as Kubernetes resource quantity strings
// (e.g. "4Gi", "500m", "8").
//
// For uniform distributions, if both min and max are whole-unit quantities (no fractional
// milli part), sampling is done at unit granularity. This ensures integer resources like
// nvidia.com/gpu always produce whole-number results (e.g. "2", not "2500m").
//
// Used for: resource requests (cpu, memory, nvidia.com/gpu).
func (s *Sampler) SampleQuantity(d *config.Distribution) (resource.Quantity, error) {
	if d.IsFixed() {
		q, err := resource.ParseQuantity(d.Value)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("fixed quantity %q: %w", d.Value, err)
		}
		return q, nil
	}

	switch d.Type {
	case "uniform":
		minQ, err := resource.ParseQuantity(d.Min)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("uniform min %q: %w", d.Min, err)
		}
		maxQ, err := resource.ParseQuantity(d.Max)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("uniform max %q: %w", d.Max, err)
		}
		minMilli := minQ.MilliValue()
		maxMilli := maxQ.MilliValue()
		if maxMilli < minMilli {
			return resource.Quantity{}, fmt.Errorf("uniform max (%v) < min (%v)", maxQ.String(), minQ.String())
		}
		var sampledMilli int64
		if minMilli%1000 == 0 && maxMilli%1000 == 0 {
			// Both endpoints are whole-unit quantities: sample at unit granularity.
			// This ensures integer resources (e.g. nvidia.com/gpu) stay whole numbers.
			minUnits := minMilli / 1000
			maxUnits := maxMilli / 1000
			sampledMilli = (minUnits + s.rng.Int63n(maxUnits-minUnits+1)) * 1000
		} else {
			spread := maxMilli - minMilli
			sampledMilli = minMilli + s.rng.Int63n(spread+1)
		}
		result := resource.NewMilliQuantity(sampledMilli, minQ.Format)
		return *result, nil

	case "normal":
		mean, err := resource.ParseQuantity(d.Mean)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("normal mean %q: %w", d.Mean, err)
		}
		stddev, err := resource.ParseQuantity(d.Stddev)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("normal stddev %q: %w", d.Stddev, err)
		}
		sample := float64(mean.MilliValue()) + float64(stddev.MilliValue())*s.rng.NormFloat64()
		if sample < 0 {
			sample = 0
		}
		result := resource.NewMilliQuantity(int64(math.Round(sample)), mean.Format)
		return *result, nil

	case "lognormal":
		mean, err := resource.ParseQuantity(d.Mean)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("lognormal mean %q: %w", d.Mean, err)
		}
		stddev, err := resource.ParseQuantity(d.Stddev)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("lognormal stddev %q: %w", d.Stddev, err)
		}
		mu, sigma := lognormalParams(float64(mean.MilliValue()), float64(stddev.MilliValue()))
		sample := math.Exp(mu + sigma*s.rng.NormFloat64())
		result := resource.NewMilliQuantity(int64(math.Round(sample)), mean.Format)
		return *result, nil

	case "choice":
		val, err := s.weightedChoice(d.Values, d.Weights)
		if err != nil {
			return resource.Quantity{}, err
		}
		q, err := resource.ParseQuantity(val)
		if err != nil {
			return resource.Quantity{}, fmt.Errorf("choice value %q: %w", val, err)
		}
		return q, nil

	default:
		return resource.Quantity{}, fmt.Errorf("unsupported distribution type %q", d.Type)
	}
}

// weightedChoice selects a value from values using optional weights.
// If weights is nil or empty, uniform selection is used.
func (s *Sampler) weightedChoice(values []string, weights []int) (string, error) {
	if len(values) == 0 {
		return "", fmt.Errorf("choice: values must not be empty")
	}
	if len(weights) == 0 {
		return values[s.rng.Intn(len(values))], nil
	}

	total := 0
	for _, w := range weights {
		total += w
	}
	if total <= 0 {
		return "", fmt.Errorf("choice: total weight must be > 0, got %d", total)
	}

	r := s.rng.Intn(total)
	cumulative := 0
	for i, w := range weights {
		cumulative += w
		if r < cumulative {
			return values[i], nil
		}
	}
	return values[len(values)-1], nil
}

// lognormalParams converts the desired lognormal mean and stddev into the
// underlying normal distribution parameters (mu, sigma).
//
// If X ~ N(mu, sigma²), then Y = e^X ~ LogNormal with:
//
//	E[Y]   = exp(mu + sigma²/2)
//	Var[Y] = (exp(sigma²) - 1) * exp(2*mu + sigma²)
//
// Solving for mu and sigma given the target mean m and stddev s:
//
//	sigma² = ln(1 + (s/m)²)
//	mu     = ln(m) - sigma²/2
//
// This is the standard parameterization used by scipy.stats.lognorm and
// numpy.random.lognormal. See: https://en.wikipedia.org/wiki/Log-normal_distribution
func lognormalParams(mean, stddev float64) (mu, sigma float64) {
	if mean <= 0 {
		return 0, 0
	}
	cv2 := (stddev / mean) * (stddev / mean)
	sigma2 := math.Log1p(cv2)
	sigma = math.Sqrt(sigma2)
	mu = math.Log(mean) - sigma2/2
	return mu, sigma
}
