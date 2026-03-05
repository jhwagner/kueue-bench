package workload

import (
	"math"
	"testing"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

func ptr[T any](v T) *T { return &v }

// TestSamplerDeterministic verifies that the same seed produces identical sequences.
func TestSamplerDeterministic(t *testing.T) {
	seed := int64(42)
	d := &config.Distribution{Type: "uniform", Min: "1", Max: "10"}

	s1 := NewSampler(&seed)
	s2 := NewSampler(&seed)

	for i := range 5 {
		v1, err1 := s1.SampleInt(d)
		v2, err2 := s2.SampleInt(d)
		if err1 != nil || err2 != nil {
			t.Fatalf("iter %d: unexpected error", i)
		}
		if v1 != v2 {
			t.Errorf("iter %d: got %d and %d, want identical values", i, v1, v2)
		}
	}
}

// TestSamplerNilSeedDoesNotPanic verifies that a nil seed is accepted.
func TestSamplerNilSeedDoesNotPanic(t *testing.T) {
	s := NewSampler(nil)
	d := &config.Distribution{Value: "5"}
	if _, err := s.SampleInt(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- SampleInt ---

func TestSampleIntFixed(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	n, err := s.SampleInt(&config.Distribution{Value: "8"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 8 {
		t.Errorf("got %d, want 8", n)
	}
}

func TestSampleIntUniform(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "uniform", Min: "2", Max: "8"}
	for range 100 {
		n, err := s.SampleInt(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n < 2 || n > 8 {
			t.Errorf("got %d, want in [2, 8]", n)
		}
	}
}

func TestSampleIntUniformMinEqualsMax(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "uniform", Min: "4", Max: "4"}
	n, err := s.SampleInt(d)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Errorf("got %d, want 4", n)
	}
}

func TestSampleIntNormal(t *testing.T) {
	s := NewSampler(ptr(int64(99)))
	d := &config.Distribution{Type: "normal", Mean: "10", Stddev: "2"}
	sum := int64(0)
	const iterations = 1000
	for range iterations {
		n, err := s.SampleInt(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sum += n
	}
	// Mean should be close to 10
	got := float64(sum) / iterations
	if math.Abs(got-10) > 1.0 {
		t.Errorf("normal mean = %.2f, want ~10 (±1.0)", got)
	}
}

func TestSampleIntLognormal(t *testing.T) {
	s := NewSampler(ptr(int64(7)))
	d := &config.Distribution{Type: "lognormal", Mean: "4", Stddev: "2"}
	for range 100 {
		n, err := s.SampleInt(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n < 0 {
			t.Errorf("lognormal produced negative int %d", n)
		}
	}
}

func TestSampleIntChoice(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "choice", Values: []string{"2", "4", "8"}}
	allowed := map[int64]bool{2: true, 4: true, 8: true}
	for range 50 {
		n, err := s.SampleInt(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed[n] {
			t.Errorf("got %d, want one of {2, 4, 8}", n)
		}
	}
}

func TestSampleIntChoiceWeighted(t *testing.T) {
	// Weight 100% on value "8"
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{
		Type:    "choice",
		Values:  []string{"2", "8"},
		Weights: []int{0, 100},
	}
	for range 20 {
		n, err := s.SampleInt(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if n != 8 {
			t.Errorf("got %d, want 8 (weight 100 on 8)", n)
		}
	}
}

func TestSampleIntErrors(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	cases := []struct {
		name string
		d    config.Distribution
	}{
		{"bad fixed", config.Distribution{Value: "not-a-number"}},
		{"bad uniform min", config.Distribution{Type: "uniform", Min: "x", Max: "4"}},
		{"bad uniform max", config.Distribution{Type: "uniform", Min: "1", Max: "x"}},
		{"uniform max < min", config.Distribution{Type: "uniform", Min: "5", Max: "2"}},
		{"bad normal mean", config.Distribution{Type: "normal", Mean: "x", Stddev: "1"}},
		{"bad lognormal stddev", config.Distribution{Type: "lognormal", Mean: "4", Stddev: "x"}},
		{"unsupported type", config.Distribution{Type: "zipf"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.SampleInt(&tc.d); err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

// --- SampleDuration ---

func TestSampleDurationFixed(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	dur, err := s.SampleDuration(&config.Distribution{Value: "30m"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if dur != 30*time.Minute {
		t.Errorf("got %v, want 30m", dur)
	}
}

func TestSampleDurationUniform(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "uniform", Min: "1h", Max: "4h"}
	for range 100 {
		dur, err := s.SampleDuration(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dur < time.Hour || dur > 4*time.Hour {
			t.Errorf("got %v, want in [1h, 4h]", dur)
		}
	}
}

func TestSampleDurationNormal(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "normal", Mean: "30m", Stddev: "5m"}
	for range 100 {
		dur, err := s.SampleDuration(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dur < 0 {
			t.Errorf("normal duration clamped to negative: %v", dur)
		}
	}
}

func TestSampleDurationLognormal(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	// Mean 20m, stddev 10m — typical ML job duration distribution
	d := &config.Distribution{Type: "lognormal", Mean: "20m", Stddev: "10m"}
	for range 100 {
		dur, err := s.SampleDuration(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if dur < 0 {
			t.Errorf("lognormal duration should be non-negative, got %v", dur)
		}
	}
}

func TestSampleDurationChoice(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "choice", Values: []string{"1h", "2h", "4h"}}
	allowed := map[time.Duration]bool{
		time.Hour: true, 2 * time.Hour: true, 4 * time.Hour: true,
	}
	for range 30 {
		dur, err := s.SampleDuration(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed[dur] {
			t.Errorf("got %v, want one of {1h, 2h, 4h}", dur)
		}
	}
}

// --- SampleQuantity ---

func TestSampleQuantityFixed(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	q, err := s.SampleQuantity(&config.Distribution{Value: "4Gi"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.String() != "4Gi" {
		t.Errorf("got %v, want 4Gi", q.String())
	}
}

func TestSampleQuantityUniformGPU(t *testing.T) {
	// GPU counts must remain whole numbers
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "uniform", Min: "1", Max: "4"}
	for range 100 {
		q, err := s.SampleQuantity(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		milli := q.MilliValue()
		if milli%1000 != 0 {
			t.Errorf("GPU quantity has fractional milli part: %v (%dm)", q.String(), milli)
		}
		units := milli / 1000
		if units < 1 || units > 4 {
			t.Errorf("GPU units %d outside [1, 4]", units)
		}
	}
}

func TestSampleQuantityUniformMemory(t *testing.T) {
	// Memory quantities with BinarySI suffixes
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "uniform", Min: "1Gi", Max: "8Gi"}
	for range 50 {
		q, err := s.SampleQuantity(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		milli := q.MilliValue()
		if milli < 1024*1024*1024*1000 || milli > 8*1024*1024*1024*1000 {
			t.Errorf("memory %v outside [1Gi, 8Gi]", q.String())
		}
	}
}

func TestSampleQuantityLognormal(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "lognormal", Mean: "4Gi", Stddev: "2Gi"}
	for range 50 {
		q, err := s.SampleQuantity(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if q.MilliValue() < 0 {
			t.Errorf("lognormal quantity should be non-negative, got %v", q.String())
		}
	}
}

func TestSampleQuantityChoice(t *testing.T) {
	s := NewSampler(ptr(int64(1)))
	d := &config.Distribution{Type: "choice", Values: []string{"64Gi", "128Gi", "256Gi"}}
	allowed := map[string]bool{"64Gi": true, "128Gi": true, "256Gi": true}
	for range 30 {
		q, err := s.SampleQuantity(d)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !allowed[q.String()] {
			t.Errorf("got %v, want one of {64Gi, 128Gi, 256Gi}", q.String())
		}
	}
}

// --- lognormalParams ---

func TestLognormalParamsMean(t *testing.T) {
	// Sample many values and verify the empirical mean is close to the target.
	const (
		targetMean   = 1200.0
		targetStddev = 600.0
		iterations   = 10000
		tolerance    = 50.0
	)
	mu, sigma := lognormalParams(targetMean, targetStddev)

	seed := int64(42)
	s := NewSampler(&seed)
	sum := 0.0
	for range iterations {
		sample := math.Exp(mu + sigma*s.rng.NormFloat64())
		sum += sample
	}
	empiricalMean := sum / iterations
	if math.Abs(empiricalMean-targetMean) > tolerance {
		t.Errorf("empirical mean = %.1f, want %.1f ± %.1f", empiricalMean, targetMean, tolerance)
	}
}

func TestLognormalParamsZeroMean(t *testing.T) {
	mu, sigma := lognormalParams(0, 1)
	if mu != 0 || sigma != 0 {
		t.Errorf("zero mean: got mu=%v sigma=%v, want 0 0", mu, sigma)
	}
}
