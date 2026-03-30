package workload

import (
	"context"
	"fmt"
	"time"

	"github.com/jhwagner/kueue-bench/pkg/config"
)

// RunResult contains summary information from a completed Engine.Run invocation.
type RunResult struct {
	WorkloadCount int
	EffectiveSeed int64
}

// Engine orchestrates workload generation according to a WorkloadProfile.
// It drives the arrival scheduler, selects workload types by weight, builds
// unstructured objects, and submits them to the cluster.
type Engine struct {
	profile   *config.WorkloadProfile
	sampler   *Sampler
	scheduler ArrivalScheduler
	client    *WorkloadClient
	runID     string
	dryRun    bool
	onSubmit  func(name, workloadType, namespace string)
}

// EngineOption configures an Engine.
type EngineOption func(*Engine)

// WithDryRun enables dry-run mode. Workloads are built but not submitted.
func WithDryRun() EngineOption {
	return func(e *Engine) { e.dryRun = true }
}

// WithOnSubmit registers a callback invoked after each workload is submitted
// (or would be, in dry-run mode). Useful for CLI progress output.
func WithOnSubmit(fn func(name, workloadType, namespace string)) EngineOption {
	return func(e *Engine) { e.onSubmit = fn }
}

// NewEngine creates an Engine from a WorkloadProfile.
// kubeconfigPath is required unless WithDryRun is set.
func NewEngine(profile *config.WorkloadProfile, kubeconfigPath, runID string, opts ...EngineOption) (*Engine, error) {
	sampler := NewSampler(profile.Spec.Seed)

	scheduler, err := NewArrivalScheduler(profile.Spec.ArrivalPattern, sampler.Rand())
	if err != nil {
		return nil, fmt.Errorf("arrival scheduler: %w", err)
	}

	e := &Engine{
		profile:   profile,
		sampler:   sampler,
		scheduler: scheduler,
		runID:     runID,
	}
	for _, opt := range opts {
		opt(e)
	}

	if !e.dryRun {
		if kubeconfigPath == "" {
			return nil, fmt.Errorf("kubeconfigPath required when not in dry-run mode")
		}
		wc, err := NewWorkloadClient(kubeconfigPath)
		if err != nil {
			return nil, fmt.Errorf("workload client: %w", err)
		}
		e.client = wc
	}

	return e, nil
}

// EffectiveSeed returns the seed used by the engine's sampler.
// This is useful for logging/persisting the seed before the run starts.
func (e *Engine) EffectiveSeed() int64 {
	return e.sampler.Seed()
}

// Run generates and submits workloads until the profile duration elapses or
// the context is cancelled. Returns a RunResult summarising the run.
func (e *Engine) Run(ctx context.Context) (RunResult, error) {
	duration, err := time.ParseDuration(e.profile.Spec.Duration)
	if err != nil {
		return RunResult{}, fmt.Errorf("profile duration %q: %w", e.profile.Spec.Duration, err)
	}

	deadlineCtx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	result := RunResult{EffectiveSeed: e.EffectiveSeed()}

	workloads := e.profile.Spec.Workloads
	weights := make([]int, len(workloads))
	for i := range workloads {
		weights[i] = workloads[i].Weight
	}

	for index := 0; ; index++ {
		interval := e.scheduler.NextInterval()

		timer := time.NewTimer(interval)
		select {
		case <-deadlineCtx.Done():
			timer.Stop()
			result.WorkloadCount = index
			return result, nil
		case <-timer.C:
		}

		spec := &workloads[e.sampler.SampleIndex(len(workloads), weights)]

		builder, err := builderFor(spec.Type)
		if err != nil {
			result.WorkloadCount = index
			return result, fmt.Errorf("build workload #%d: %w", index, err)
		}

		obj, gvr, err := builder.Build(spec, e.profile.Metadata.Name, e.runID, index, e.sampler)
		if err != nil {
			result.WorkloadCount = index
			return result, fmt.Errorf("build workload #%d: %w", index, err)
		}

		if !e.dryRun {
			if err := e.client.Create(deadlineCtx, gvr, obj); err != nil {
				result.WorkloadCount = index
				if deadlineCtx.Err() != nil {
					// Profile duration elapsed during the API call; treat as clean termination.
					return result, nil
				}
				return result, fmt.Errorf("submit workload #%d: %w", index, err)
			}
		}

		if e.onSubmit != nil {
			e.onSubmit(obj.GetName(), spec.Type, obj.GetNamespace())
		}
	}
}
