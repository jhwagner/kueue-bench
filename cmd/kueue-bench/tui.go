package main

import (
	"fmt"
	"sort"

	tea "charm.land/bubbletea/v2"
	"github.com/spf13/cobra"

	"github.com/jhwagner/kueue-bench/pkg/topology"
	pkgtui "github.com/jhwagner/kueue-bench/pkg/tui"
)

var (
	tuiTopology string
	tuiCluster  string
)

var tuiCmd = &cobra.Command{
	Use:   "tui",
	Short: "Interactive TUI for exploring a topology",
	Long: `Launch an interactive terminal UI connected to a running topology.

The TUI shows real-time queue utilization, workload status, and admission events.
Run 'workload submit' in a separate terminal to observe workloads flowing through.`,
	RunE: runTUI,
}

func init() {
	tuiCmd.Flags().StringVarP(&tuiTopology, "topology", "t", "", "topology name (required)")
	tuiCmd.Flags().StringVar(&tuiCluster, "cluster", "", "cluster to connect to (default: management or only cluster)")
	_ = tuiCmd.MarkFlagRequired("topology")
	rootCmd.AddCommand(tuiCmd)
}

func runTUI(cmd *cobra.Command, args []string) error {
	topo, err := topology.Load(tuiTopology)
	if err != nil {
		return fmt.Errorf("load topology %q: %w", tuiTopology, err)
	}
	meta := *topo.GetMetadata()

	clusterName, err := resolveCluster(meta, tuiTopology, tuiCluster)
	if err != nil {
		return err
	}

	m, err := pkgtui.New(tuiTopology, clusterName, meta)
	if err != nil {
		return fmt.Errorf("create TUI: %w", err)
	}

	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		return fmt.Errorf("TUI error: %w", err)
	}
	return nil
}

// resolveCluster picks the default cluster when --cluster is not specified:
// 1. A cluster with role="management" is preferred.
// 2. A cluster named after the topology (legacy MultiKueue convention).
// 3. If there's only one cluster, use it.
// 4. Otherwise the user must specify --cluster.
func resolveCluster(meta topology.Metadata, topoName, clusterName string) (string, error) {
	if clusterName != "" {
		if _, ok := meta.Clusters[clusterName]; !ok {
			return "", fmt.Errorf("cluster %q not found in topology (available: %v)",
				clusterName, sortedClusterNames(meta.Clusters))
		}
		return clusterName, nil
	}

	// Prefer management role.
	for name, c := range meta.Clusters {
		if c.Role == "management" {
			return name, nil
		}
	}

	// Legacy: cluster named after topology.
	if _, ok := meta.Clusters[topoName]; ok {
		return topoName, nil
	}

	// Single cluster.
	if len(meta.Clusters) == 1 {
		for name := range meta.Clusters {
			return name, nil
		}
	}

	return "", fmt.Errorf("topology has multiple clusters; use --cluster to specify one of: %v",
		sortedClusterNames(meta.Clusters))
}

func sortedClusterNames(clusters map[string]topology.Cluster) []string {
	names := make([]string, 0, len(clusters))
	for name := range clusters {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
