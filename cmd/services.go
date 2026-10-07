package cmd

import (
	"fmt"
	"net/url"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var servicesCmd = &cobra.Command{
	Use:     "services",
	Aliases: []string{"service"},
	Short:   "List and restart system services",
	Long: `Inspect or restart the system services managed by Cipi (nginx, PHP-FPM,
MariaDB, PostgreSQL, Supervisor, fail2ban, …).

Requires Cipi 5.0.6+ (API 1.15+) and services-view / services-manage abilities.

  cipi-cli services list
  cipi-cli services list nginx
  cipi-cli services restart nginx

` + multiServerTip,
	Example: `  cipi-cli services list
  cipi-cli prod services restart php8.4-fpm`,
}

var servicesListCmd = &cobra.Command{
	Use:   "list [name]",
	Short: "List system services and their status",
	Long: `List system services with status and uptime. Pass a service name to show
only that one.

  cipi-cli services list
  cipi-cli services list nginx --json`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		path := "/api/services"
		if len(args) == 1 {
			path += "?service=" + url.QueryEscape(args[0])
		}

		var result struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to list services: %s", apiErrorHint(err, "1.15.0", "services-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(result.Data) == 0 {
			output.Warn("No services found")
			return nil
		}

		output.Header("Services")
		t := output.NewTable("SERVICE", "STATUS", "SINCE")
		for _, s := range result.Data {
			t.Row(str(s, "name"), colorStatus(str(s, "status")), str(s, "since"))
		}
		t.Flush()
		return nil
	},
}

var servicesRestartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Restart a system service (async job)",
	Long: `Restart one system service by the name shown in 'services list'
(e.g. nginx, mariadb, supervisor).

  cipi-cli services restart nginx
  cipi-cli prod services restart php8.4-fpm`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Restarting service '%s'...", args[0])
		path := fmt.Sprintf("/api/services/%s/restart", url.PathEscape(args[0]))
		if err := client.DoAsyncAndWait("POST", path, nil); err != nil {
			output.Error("Service restart failed: %s", apiErrorHint(err, "1.15.0", "services-manage"))
			return err
		}

		output.Success("Service '%s' restarted", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	servicesCmd.AddCommand(servicesListCmd, servicesRestartCmd)
	rootCmd.AddCommand(servicesCmd)
}
