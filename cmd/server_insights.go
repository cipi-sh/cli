package cmd

import (
	"fmt"

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
	Short: "Show Zero Trust status (Cloudflare Tunnel, Tailscale, NetBird)",
	Long: `Read-only Zero Trust status from GET /api/zt.

Cloudflare Tunnel publishes the sites; a mesh network (Tailscale or NetBird)
is how the people who run the server reach it. Both are shown: the tunnel and
its locks, and the mesh — connected or not, the server's address on it, peers,
the node key's expiry, how to SSH in over it, and its own locks.

--json prints everything the server said, including hostnames, Access,
service tokens and TCP services.

Requires Cipi 5.3.0+ and zt-view ability; the mesh needs Cipi 5.18.0+ and
cipi/api 1.48+. Every change stays on the server: cipi zt, cipi zt tailscale,
cipi zt netbird.`,
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

		// An older server has no details: what it sends is printed as it was.
		data, _ := result.Data.(map[string]interface{})
		if data == nil || !ztHasDetails(data) {
			output.Header("Cloudflare Zero Trust")
			printDataWrapper(result.Data)
			return nil
		}

		output.Header("Zero Trust — Cloudflare Tunnel")
		for _, row := range ztCloudflareRows(data) {
			output.KeyValue(nil, row.Label, row.Value)
		}
		if rows := ztMeshRows(data); rows != nil {
			output.Header("Zero Trust — mesh network")
			for _, row := range rows {
				if row.Label == "warning" {
					output.Warn("%s", row.Value)
					continue
				}
				output.KeyValue(nil, row.Label, row.Value)
			}
		}
		fmt.Println()
		output.Dim.Println("  Hostnames, Access, service tokens and TCP services: cipi-cli zt --json")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(monitorCmd, ztCmd)
}
