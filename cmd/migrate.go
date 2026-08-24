package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/internal/config/legacy"
)

// NewMigrateCmd returns the "migrate" command.
func NewMigrateCmd() *cobra.Command {
	var (
		inputFile  string
		outputFile string
	)

	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Migrate a legacy v1 harness config to v1alpha1",
		Long: `Migrate converts a legacy v1 harness config to the new v1alpha1 format.

The input YAML is parsed, normalized, and written as v1alpha1 YAML to stdout or
a file. Fields that have no v1alpha1 home (task, include, inline policy
documents, unresolved base_agent) are reported as warnings on stderr.`,
		Example: `  harness migrate -f old-agent.yaml > harness.yaml
  harness migrate -f old.yaml -o new-harness.yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if inputFile == "" {
				return fmt.Errorf("flag -f/--file is required")
			}

			// Read the legacy file
			data, err := os.ReadFile(inputFile)
			if err != nil {
				return fmt.Errorf("reading %s: %w", inputFile, err)
			}

			// Migrate
			output, warnings, err := legacy.MigrateBytes(data)
			if err != nil {
				return fmt.Errorf("migrating: %w", err)
			}

			// Write output
			if outputFile != "" {
				if err := os.WriteFile(outputFile, output, 0o644); err != nil {
					return fmt.Errorf("writing %s: %w", outputFile, err)
				}
			} else {
				// Write to command's Out (which can be a buffer in tests)
				if _, err := cmd.OutOrStdout().Write(output); err != nil {
					return fmt.Errorf("writing output: %w", err)
				}
			}

			// Print warnings to command's ErrOrStderr (which can be a buffer in
			// tests). Migration warnings flag data that has no v1alpha1 home, so
			// a failed write must not be swallowed into a silent success.
			for _, w := range warnings {
				if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "warning: %s: %s\n", w.Field, w.Message); err != nil {
					return fmt.Errorf("writing migration warnings: %w", err)
				}
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&inputFile, "file", "f", "", "Path to legacy harness YAML (required)")
	cmd.Flags().StringVarP(&outputFile, "output", "o", "", "Path to write v1alpha1 YAML (default: stdout)")

	return cmd
}
