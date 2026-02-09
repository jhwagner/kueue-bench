package kwok

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/jhwagner/kueue-bench/pkg/manifest"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/restmapper"
)

// Embedded stage manifests from the stages/ directory.
var (
	//go:embed stages/node-heartbeat-with-lease.yaml
	nodeHeartbeatStage []byte

	//go:embed stages/node-initialize.yaml
	nodeInitializeStage []byte

	//go:embed stages/pod-ready.yaml
	podReadyStage []byte

	//go:embed stages/pod-delete.yaml
	podDeleteStage []byte

	//go:embed stages/pod-complete-timed.yaml
	podCompleteTimedStage []byte

	//go:embed stages/pod-complete-manual.yaml
	podCompleteManualStage []byte
)

// installStages applies all embedded Kwok stages to the cluster.
func installStages(ctx context.Context, dynamicClient dynamic.Interface,
	mapper *restmapper.DeferredDiscoveryRESTMapper) error {

	stages := [][]byte{
		nodeHeartbeatStage,
		nodeInitializeStage,
		podReadyStage,
		podDeleteStage,
		podCompleteTimedStage,
		podCompleteManualStage,
	}

	for _, stage := range stages {
		if err := manifest.ApplyBytes(ctx, dynamicClient, mapper, stage); err != nil {
			return fmt.Errorf("failed to apply Kwok stage: %w", err)
		}
	}

	return nil
}
