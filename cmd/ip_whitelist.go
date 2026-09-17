package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var ipWhitelistCmd = &cobra.Command{
	Use:     "ip-whitelist",
	Aliases: []string{"ipwhitelist"},
	Short:   "Manage API client IP whitelist",
	Long: `View or change the optional IP allowlist for /api/* and /mcp.

Requires Cipi 5.0.8+ and ip-whitelist-view / ip-whitelist-manage abilities.

  cipi-cli ip-whitelist show
  cipi-cli ip-whitelist set 203.0.113.10,10.0.0.0/8
  cipi-cli ip-whitelist add 203.0.113.10
  cipi-cli ip-whitelist remove 203.0.113.10
  cipi-cli ip-whitelist allow-all

` + multiServerTip,
}

var ipWhitelistShowCmd = &cobra.Command{
	Use:   "show",
	Short: "Show current IP whitelist",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/ip-whitelist", &result); err != nil {
			output.Error("Failed to read IP whitelist: %s", apiErrorHint(err, "5.0.8", "ip-whitelist-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("API IP whitelist")
		printDataWrapper(result.Data)
		return nil
	},
}

var ipWhitelistSetCmd = &cobra.Command{
	Use:   "set <entries...>",
	Short: "Replace the IP whitelist",
	Long: `Replace whitelist entries. Pass comma-separated values or multiple args.
Use '*' to allow all clients.

  cipi-cli ip-whitelist set 203.0.113.10,10.0.0.0/8
  cipi-cli ip-whitelist set '*'`,
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		entries := splitEntries(strings.Join(args, ","))
		noEnsure, _ := cmd.Flags().GetBool("no-ensure-client-ip")

		body := map[string]interface{}{
			"entries": entries,
		}
		if noEnsure {
			body["ensure_client_ip"] = false
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put("/api/ip-whitelist", body, &result); err != nil {
			output.Error("Failed to set IP whitelist: %s", apiErrorHint(err, "5.0.8", "ip-whitelist-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("IP whitelist updated")
		fmt.Println()
		return nil
	},
}

var ipWhitelistAddCmd = &cobra.Command{
	Use:   "add <ip>",
	Short: "Add an IP or CIDR to the whitelist",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"ip": args[0]}
		if err := client.Post("/api/ip-whitelist", body, nil); err != nil {
			output.Error("Failed to add IP: %s", apiErrorHint(err, "5.0.8", "ip-whitelist-manage"))
			return err
		}

		output.Success("Added %s to IP whitelist", args[0])
		fmt.Println()
		return nil
	},
}

var ipWhitelistRemoveCmd = &cobra.Command{
	Use:   "remove <ip>",
	Short: "Remove an IP or CIDR from the whitelist",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"ip": args[0]}
		if err := client.Delete("/api/ip-whitelist", body, nil); err != nil {
			output.Error("Failed to remove IP: %s", apiErrorHint(err, "5.0.8", "ip-whitelist-manage"))
			return err
		}

		output.Success("Removed %s from IP whitelist", args[0])
		fmt.Println()
		return nil
	},
}

var ipWhitelistAllowAllCmd = &cobra.Command{
	Use:   "allow-all",
	Short: "Allow all client IPs",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post("/api/ip-whitelist/allow-all", nil, nil); err != nil {
			output.Error("Failed to allow all IPs: %s", apiErrorHint(err, "5.0.8", "ip-whitelist-manage"))
			return err
		}

		output.Success("API IP whitelist set to allow all")
		fmt.Println()
		return nil
	},
}

func splitEntries(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func init() {
	ipWhitelistSetCmd.Flags().Bool("no-ensure-client-ip", false, "Do not auto-append caller IP when restricting")

	ipWhitelistCmd.AddCommand(
		ipWhitelistShowCmd, ipWhitelistSetCmd,
		ipWhitelistAddCmd, ipWhitelistRemoveCmd, ipWhitelistAllowAllCmd,
	)
	rootCmd.AddCommand(ipWhitelistCmd)
}
