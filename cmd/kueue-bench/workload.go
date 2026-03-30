package main

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/run"
	"github.com/jhwagner/kueue-bench/pkg/topology"
	"github.com/jhwagner/kueue-bench/pkg/workload"
)

var workloadCmd = &cobra.Command{
	Use:   "workload",
	Short: "Manage workload submissions",
	Long:  `Submit and manage workloads against Kueue topologies.`,
}

var workloadSubmitCmd = &cobra.Command{
	Use:   "submit",
	Short: "Submit workloads to a topology",
	Long: `Submit workloads to a Kueue topology according to a WorkloadProfile.

The WorkloadProfile defines workload types (Job, JobSet, RayJob), their arrival
pattern (constant or Poisson), relative weights, and resource distributions.

Examples:
  kueue-bench workload submit --topology my-cluster --profile ml-training-mix.yaml
  kueue-bench workload submit --topology my-cluster --profile profile.yaml --dry-run`,
	RunE: runWorkloadSubmit,
}

var (
	workloadProfileFile string
	workloadTopology    string
	workloadCluster     string
	workloadDryRun      bool
)

func init() {
	rootCmd.AddCommand(workloadCmd)
	workloadCmd.AddCommand(workloadSubmitCmd)

	workloadSubmitCmd.Flags().StringVarP(&workloadProfileFile, "profile", "p", "", "path to workload profile file (required)")
	workloadSubmitCmd.Flags().StringVar(&workloadTopology, "topology", "", "topology name (required unless --dry-run)")
	workloadSubmitCmd.Flags().StringVar(&workloadCluster, "cluster", "", "cluster name within the topology (default: management cluster)")
	workloadSubmitCmd.Flags().BoolVar(&workloadDryRun, "dry-run", false, "build workloads and print them without submitting")

	_ = workloadSubmitCmd.MarkFlagRequired("profile")
}

func runWorkloadSubmit(cmd *cobra.Command, _ []string) error {
	// Load and validate workload profile
	profile, err := config.LoadWorkloadProfile(workloadProfileFile)
	if err != nil {
		return fmt.Errorf("failed to load workload profile: %w", err)
	}
	if err := config.ValidateWorkloadProfile(profile); err != nil {
		return fmt.Errorf("invalid workload profile: %w", err)
	}

	// Resolve kubeconfig path from topology metadata
	kubeconfigPath := ""
	if !workloadDryRun {
		if workloadTopology == "" {
			return fmt.Errorf("--topology is required when not using --dry-run")
		}
		kubeconfigPath, err = resolveKubeconfigPath(workloadTopology, workloadCluster)
		if err != nil {
			return err
		}
	}

	runID := generateRunID()
	startedAt := time.Now()

	opts := []workload.EngineOption{
		workload.WithOnSubmit(func(name, workloadType, namespace string) {
			fmt.Printf("  %s/%s (%s)\n", namespace, name, workloadType)
		}),
	}
	if workloadDryRun {
		opts = append(opts, workload.WithDryRun())
	}

	engine, err := workload.NewEngine(profile, kubeconfigPath, runID, opts...)
	if err != nil {
		return fmt.Errorf("failed to create engine: %w", err)
	}

	fmt.Printf("Submitting workloads from profile %q (run ID: %s, seed: %d)\n",
		profile.Metadata.Name, runID, engine.EffectiveSeed())
	if workloadDryRun {
		fmt.Println("(dry-run mode: workloads will not be submitted)")
	}

	result, err := engine.Run(cmd.Context())
	if err != nil {
		return fmt.Errorf("workload generation failed: %w", err)
	}

	elapsed := time.Since(startedAt)
	fmt.Printf("Workload generation complete: %d workloads in %s (run ID: %s)\n",
		result.WorkloadCount, elapsed.Round(time.Millisecond), runID)

	// Persist run metadata (best-effort)
	profilePath, _ := filepath.Abs(workloadProfileFile)
	meta := &run.RunMetadata{
		RunID:         runID,
		ProfileName:   profile.Metadata.Name,
		ProfilePath:   profilePath,
		TopologyName:  workloadTopology,
		ClusterName:   workloadCluster,
		Seed:          result.EffectiveSeed,
		DryRun:        workloadDryRun,
		WorkloadCount: result.WorkloadCount,
		StartedAt:     startedAt,
		Duration:      elapsed.Round(time.Millisecond).String(),
	}
	if err := run.Save(meta); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save run metadata: %v\n", err)
	}

	return nil
}

// resolveKubeconfigPath returns the kubeconfig path for the target cluster within a topology.
// If clusterName is empty, the target is inferred:
//  1. A cluster named after the topology (MultiKueue management cluster) is preferred.
//  2. If no such cluster exists but the topology has exactly one cluster, that cluster is used.
//  3. Otherwise --cluster must be specified explicitly.
func resolveKubeconfigPath(topologyName, clusterName string) (string, error) {
	topo, err := topology.Load(topologyName)
	if err != nil {
		return "", fmt.Errorf("failed to load topology %q: %w", topologyName, err)
	}

	meta := topo.GetMetadata()

	if clusterName == "" {
		if _, ok := meta.Clusters[topologyName]; ok {
			// MultiKueue topology: management cluster is named after the topology.
			clusterName = topologyName
		} else if len(meta.Clusters) == 1 {
			// Single-cluster topology: only one choice.
			for name := range meta.Clusters {
				clusterName = name
			}
		} else {
			return "", fmt.Errorf("topology %q has multiple clusters; use --cluster to specify one of: %v",
				topologyName, clusterNames(meta.Clusters))
		}
	}

	cluster, ok := meta.Clusters[clusterName]
	if !ok {
		return "", fmt.Errorf("cluster %q not found in topology %q (available: %v)",
			clusterName, topologyName, clusterNames(meta.Clusters))
	}
	return cluster.KubeconfigPath, nil
}

// clusterNames returns the cluster name list for error messages.
func clusterNames(clusters map[string]topology.Cluster) []string {
	names := make([]string, 0, len(clusters))
	for name := range clusters {
		names = append(names, name)
	}
	return names
}

// generateRunID returns a short random lowercase alphanumeric identifier.
// Uses math/rand directly (not the profile seed) so run IDs are unique across reruns of the same profile.
func generateRunID() string {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 8)
	for i := range b {
		b[i] = chars[rand.Intn(len(chars))] //nolint:gosec // run ID is non-security-sensitive
	}
	return string(b)
}
