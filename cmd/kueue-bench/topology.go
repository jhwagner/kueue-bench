package main

import (
	"fmt"

	"github.com/jhwagner/kueue-bench/pkg/config"
	"github.com/jhwagner/kueue-bench/pkg/topology"
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
	cfg, err := config.LoadTopology(topologyFile)
	if err != nil {
		return fmt.Errorf("failed to load topology: %w", err)
	}

	if err := config.ValidateTopology(cfg); err != nil {
		return fmt.Errorf("topology validation failed: %w", err)
	}

	fmt.Println("✓ Topology loaded and validated")

	// Create topology (creates clusters, installs components, saves metadata)
	if _, err := topology.Create(cmd.Context(), name, cfg); err != nil {
		return fmt.Errorf("failed to create topology: %w", err)
	}

	fmt.Printf("✓ Topology '%s' created successfully\n", name)
	return nil
}

func runTopologyDelete(cmd *cobra.Command, args []string) error {
	name := args[0]
	fmt.Printf("Deleting topology '%s'...\n", name)

	// Load topology metadata
	topo, err := topology.Load(name)
	if err != nil {
		return fmt.Errorf("failed to load topology: %w", err)
	}

	// Delete topology (deletes clusters and metadata)
	if err := topo.Delete(cmd.Context()); err != nil {
		return fmt.Errorf("failed to delete topology: %w", err)
	}

	fmt.Printf("✓ Topology '%s' deleted successfully\n", name)
	return nil
}

func runTopologyList(cmd *cobra.Command, args []string) error {
	topologies, err := topology.List()
	if err != nil {
		return fmt.Errorf("failed to list topologies: %w", err)
	}

	if len(topologies) == 0 {
		fmt.Println("No topologies found")
		return nil
	}

	fmt.Printf("%-20s %-10s %-20s\n", "NAME", "CLUSTERS", "CREATED")
	fmt.Println("------------------------------------------------------------")
	for _, topo := range topologies {
		metadata := topo.GetMetadata()
		fmt.Printf("%-20s %-10d %-20s\n",
			metadata.Name,
			len(metadata.Clusters),
			metadata.CreatedAt.Format("2006-01-02 15:04:05"))
	}

	return nil
}
