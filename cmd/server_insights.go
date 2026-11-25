package cmd

import (
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var monitorCmd = &cobra.Command{
	Use:   "monitor",
	Short: "Show system monitor checks and state",
	Long: `Read-only host monitor data from GET /api/monitor.

Requires Cipi 5.3.0+ and monitor-view ability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get("/api/monitor", &result); err != nil {
			output.Error("Failed to read monitor data: %s", apiErrorHint(err, "5.3.0", "monitor-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("System monitor")
		printDataWrapper(result.Data)
		return nil
	},
}

var ztCmd = &cobra.Command{
	Use:   "zt",
	Short: "Show Cloudflare Zero Trust status",
	Long: `Read-only Zero Trust status from GET /api/zt.

Requires Cipi 5.3.0+ and zt-view ability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get("/api/zt", &result); err != nil {
			output.Error("Failed to read Zero Trust status: %s", apiErrorHint(err, "5.3.0", "zt-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Cloudflare Zero Trust")
		printDataWrapper(result.Data)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd, ztCmd)
}
