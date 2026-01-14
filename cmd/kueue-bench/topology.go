package main

import (
	"fmt"

	"github.com/jhwagner/kueue-bench/pkg/cluster"
	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/kwok"
	"github.com/spf13/cobra"
)

var topologyCmd = &cobra.Command{
	Use:   "topology",
	Short: "Manage Kueue topologies",
	Long:  `Create, delete, and list Kueue test topologies.`,
}

var topologyCreateCmd = &cobra.Command{
	Use:   "create [name]",
	Short: "Create a new topology",
	Long: `Create a new Kueue test topology from a configuration file.

This will:
  1. Create kind cluster(s)
  2. Install KWOK for node simulation
  3. Install Kueue
  4. Apply Kueue configuration objects`,
	Args: cobra.ExactArgs(1),
	RunE: runTopologyCreate,
}

var topologyDeleteCmd = &cobra.Command{
	Use:   "delete [name]",
	Short: "Delete a topology",
	Long:  `Delete a Kueue test topology and clean up all associated resources.`,
	Args:  cobra.ExactArgs(1),
	RunE:  runTopologyDelete,
}

var topologyListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all topologies",
	Long:  `List all created Kueue test topologies.`,
	RunE:  runTopologyList,
}

var (
	topologyFile string
)

func init() {
	rootCmd.AddCommand(topologyCmd)
	topologyCmd.AddCommand(topologyCreateCmd)
	topologyCmd.AddCommand(topologyDeleteCmd)
	topologyCmd.AddCommand(topologyListCmd)

	// Flags for create command
	topologyCreateCmd.Flags().StringVarP(&topologyFile, "file", "f", "", "path to topology configuration file (required)")
	topologyCreateCmd.MarkFlagRequired("file")
}

func runTopologyCreate(cmd *cobra.Command, args []string) error {
	name := args[0]
	fmt.Printf("Creating topology '%s' from file '%s'...\n", name, topologyFile)

	// Load and validate topology configuration
	topology, err := config.LoadTopology(topologyFile)
	if err != nil {
		return fmt.Errorf("failed to load topology: %w", err)
	}

	if err := config.ValidateTopology(topology); err != nil {
		return fmt.Errorf("topology validation failed: %w", err)
	}

	fmt.Println("✓ Topology loaded and validated")

	// Create kind cluster(s)
	for _, clusterCfg := range topology.Spec.Clusters {
		if err := cluster.CreateCluster(cmd.Context(), &clusterCfg); err != nil {
			return fmt.Errorf("failed to create cluster '%s': %w", clusterCfg.Name, err)
		}

		// Get kubeconfig path for this cluster
		kubeconfigPath, err := cluster.GetKubeconfigPath(clusterCfg.Name)
		if err != nil {
			return fmt.Errorf("failed to get kubeconfig for cluster '%s': %w", clusterCfg.Name, err)
		}

		// Get Kwok version
		kwokVersion := kwok.DefaultKwokVersion
		if topology.Spec.Defaults != nil && topology.Spec.Defaults.Kwok != nil && topology.Spec.Defaults.Kwok.Version != "" {
			kwokVersion = topology.Spec.Defaults.Kwok.Version
		}

		// Install Kwok
		if err := kwok.Install(cmd.Context(), kubeconfigPath, kwokVersion); err != nil {
			return fmt.Errorf("failed to install Kwok in cluster '%s': %w", clusterCfg.Name, err)
		}

		// Create Kwok nodes
		if err := kwok.CreateNodes(cmd.Context(), kubeconfigPath, clusterCfg.NodePools); err != nil {
			return fmt.Errorf("failed to create nodes in cluster '%s': %w", clusterCfg.Name, err)
		}
	}

	// TODO: Install Kueue, apply Kueue objects

	return nil
}

func runTopologyDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	fmt.Printf("Deleting topology '%s'...\n", name)
	fmt.Println("Not implemented yet")
	return nil
}

func runTopologyList(cmd *cobra.Command, args []string) error {
	fmt.Println("Listing topologies...")
	fmt.Println("Not implemented yet")
	return nil
}
