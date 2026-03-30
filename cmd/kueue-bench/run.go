package main

import (
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/jhwagner/kueue-bench/pkg/run"
)

var runCmd = &cobra.Command{
	Use:   "run",
	Short: "Manage simulation runs",
	Long:  `View and manage workload simulation runs.`,
}

var runListCmd = &cobra.Command{
	Use:   "list",
	Short: "List past workload runs",
	Long:  `List all saved workload simulation runs.`,
	RunE:  runRunList,
}

func init() {
	rootCmd.AddCommand(runCmd)
	runCmd.AddCommand(runListCmd)
}

func runRunList(_ *cobra.Command, _ []string) error {
	runs, err := run.List()
	if err != nil {
		return fmt.Errorf("failed to list runs: %w", err)
	}

	if len(runs) == 0 {
		fmt.Println("No runs found")
		return nil
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	_, _ = fmt.Fprintln(w, "RUN ID\tPROFILE\tTOPOLOGY\tSEED\tWORKLOADS\tSTARTED\tDURATION")
	_, _ = fmt.Fprintln(w, "------\t-------\t--------\t----\t---------\t-------\t--------")
	for _, r := range runs {
		topoDisplay := r.TopologyName
		if topoDisplay == "" {
			topoDisplay = "(dry-run)"
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%d\t%s\t%s\n",
			r.RunID,
			r.ProfileName,
			topoDisplay,
			r.Seed,
			r.WorkloadCount,
			r.StartedAt.Format("2006-01-02 15:04:05"),
			r.Duration,
		)
	}
	_ = w.Flush()

	return nil
}
