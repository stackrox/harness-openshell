package cmd

import (
	"fmt"

	"github.com/robbycochran/harness-openshell/internal/gateway"
	"github.com/robbycochran/harness-openshell/internal/status"
	"github.com/spf13/cobra"
)

func NewStatusCmd(harnessDir, cli string) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show sandbox and gateway status",
		RunE: func(cmd *cobra.Command, args []string) error {
			gw := gateway.New(cli)
			return runStatus(gw)
		},
	}
}

func runStatus(gw gateway.Gateway) error {
	status.Header("Gateway")
	active := gw.ActiveGateway()
	if active != "" {
		status.OKf("Active: %s", active)
		ver := gw.CLIVersion()
		if ver != "" {
			status.Infof("CLI: %s", ver)
		}
	} else {
		status.Info("No active gateway")
	}

	fmt.Println()
	status.Header("Sandboxes")
	infos, err := gw.SandboxStatus()
	if err != nil {
		if active == "" {
			status.Info("No active gateway, cannot list sandboxes")
			return nil
		}
		return fmt.Errorf("listing sandboxes: %w", err)
	}
	if len(infos) == 0 {
		status.Info("None running")
		return nil
	}

	headers := []string{"NAME", "PHASE"}
	var rows [][]string
	for _, info := range infos {
		rows = append(rows, []string{info.Name, info.Phase})
	}
	status.Table(headers, rows)
	return nil
}
