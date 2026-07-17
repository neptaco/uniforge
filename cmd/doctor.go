package cmd

import "github.com/spf13/cobra"

var doctorCmd = &cobra.Command{
	Use:   "doctor [project]",
	Short: "Diagnose local UniForge and Unity state",
	Long: `Diagnose transient Unity state that can block Editor startup or batch mode.

The project defaults to the current directory and may also be specified by
path, Unity Hub project name, or Unity Hub project index. Diagnosis is
read-only by default. With --fix, only verified stale state is repaired.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runDoctor,
}

func init() {
	rootCmd.AddCommand(doctorCmd)
	addDoctorFlags(doctorCmd)
}
