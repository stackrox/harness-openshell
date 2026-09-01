package main

import (
	"context"
	_ "embed"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/stackrox/harness-openshell/cmd"
	"github.com/stackrox/harness-openshell/internal/openshell/sdkclient"
	"github.com/stackrox/harness-openshell/internal/status"
)

var version = "dev"

//go:embed profiles/harness-basic.yaml
var defaultHarnessConfig []byte

func main() {
	var verbose, showCommands bool

	root := &cobra.Command{
		Use:           "harness",
		Short:         "OpenShell Harness — deploy and manage AI agent sandboxes",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			status.Verbose = verbose
			status.ShowCommands = showCommands
		},
	}
	root.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Show external commands")
	root.PersistentFlags().BoolVar(&showCommands, "show-commands", false, "Show external commands on stdout")

	cmd.Version = version
	root.CompletionOptions.HiddenDefaultCmd = true

	root.AddCommand(
		cmd.NewApplyCmd(sdkclient.New),
		cmd.NewGetCmd(sdkclient.New),
		cmd.NewDescribeCmd(sdkclient.New),
		cmd.NewDeleteCmd(sdkclient.New),
		cmd.NewDoctorCmd(defaultHarnessConfig, sdkclient.New),
		cmd.NewInitCmd(defaultHarnessConfig),
		cmd.NewPlanCmd(sdkclient.New),
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := root.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
