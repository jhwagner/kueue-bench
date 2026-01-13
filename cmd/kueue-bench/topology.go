package main

import (
	"fmt"

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
	fmt.Println("Not implemented yet")
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
