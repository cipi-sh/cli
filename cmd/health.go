package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var healthCmd = &cobra.Command{
	Use:     "health",
	Aliases: []string{"healthcheck", "healthchecks"},
	Short:   "Manage app HTTP healthchecks",
	Long: `Configure the HTTP healthchecks Cipi runs against apps (alerts go out via
'cipi-cli smtp').

Requires Cipi 5.0.7+ (API 1.15+) and health-view / health-manage abilities.

  cipi-cli health list
  cipi-cli health show myapp
  cipi-cli health set myapp --url https://example.com/up --expect 200
  cipi-cli health check myapp
  cipi-cli health unset myapp

` + multiServerTip,
	Example: `  cipi-cli health list
  cipi-cli prod health set myapp --url https://example.com/up
  cipi-cli health check myapp`,
}

var healthListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configured healthchecks",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/health", &result); err != nil {
			output.Error("Failed to list healthchecks: %s", apiErrorHint(err, "1.15.0", "health-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(result.Data) == 0 {
			output.Warn("No healthchecks configured — use 'cipi-cli health set <app>'")
			return nil
		}

		output.Header("Healthchecks")
		t := output.NewTable("APP", "URL", "EXPECT", "STATE", "FAILS")
		for _, h := range result.Data {
			t.Row(str(h, "app"), str(h, "url"), str(h, "expect"), colorStatus(str(h, "state")), str(h, "failcount"))
		}
		t.Flush()
		return nil
	},
}

var healthShowCmd = &cobra.Command{
	Use:   "show <app>",
	Short: "Show the healthcheck of an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/health"), &result); err != nil {
			output.Error("Failed to read healthcheck: %s", apiErrorHint(err, "1.15.0", "health-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		printHealth(args[0], result.Data)
		return nil
	},
}

var healthSetCmd = &cobra.Command{
	Use:   "set <app>",
	Short: "Enable or update the healthcheck of an app",
	Long: `Enable or update the HTTP healthcheck of an app.

Without --url, https://<primary domain>/up is checked (wildcard-domain apps
need an explicit --url). --expect is the HTTP status considered healthy
(default 200).

  cipi-cli health set myapp
  cipi-cli health set myapp --url https://example.com/up --expect 204`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]interface{}{}
		if v, _ := cmd.Flags().GetString("url"); v != "" {
			body["url"] = v
		}
		if cmd.Flags().Changed("expect") {
			v, _ := cmd.Flags().GetInt("expect")
			body["expect"] = v
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put(appAPIPath(args[0], "/health"), body, &result); err != nil {
			output.Error("Failed to set healthcheck: %s", apiErrorHint(err, "1.15.0", "health-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Healthcheck saved for '%s'", args[0])
		printHealth(args[0], result.Data)
		return nil
	},
}

var healthUnsetCmd = &cobra.Command{
	Use:     "unset <app>",
	Aliases: []string{"disable", "remove"},
	Short:   "Disable the healthcheck of an app",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Delete(appAPIPath(args[0], "/health"), nil, &result); err != nil {
			output.Error("Failed to disable healthcheck: %s", apiErrorHint(err, "1.15.0", "health-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Healthcheck disabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var healthCheckCmd = &cobra.Command{
	Use:   "check <app>",
	Short: "Run the healthcheck of an app now",
	Long: `Run the configured healthcheck immediately and print the result.

Exits with status 1 when the app is unhealthy, so it can be used in scripts.

  cipi-cli health check myapp
  cipi-cli health check myapp --json`,
	Args:          cobra.ExactArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post(appAPIPath(args[0], "/health/check"), nil, &result); err != nil {
			output.Error("Healthcheck failed to run: %s", apiErrorHint(err, "1.15.0", "health-view"))
			return err
		}

		ok, _ := result.Data["ok"].(bool)
		if jsonFlag {
			output.PrintJSON(result)
		} else {
			fmt.Println()
			if ok {
				output.Success("'%s' is healthy", args[0])
			} else {
				output.Error("'%s' is unhealthy", args[0])
			}
			output.KeyValue(nil, "URL", str(result.Data, "url"))
			output.KeyValue(nil, "Expected", str(result.Data, "expect"))
			output.KeyValue(nil, "Got", str(result.Data, "got"))
			fmt.Println()
		}

		if !ok {
			return fmt.Errorf("healthcheck failed for %s", args[0])
		}
		return nil
	},
}

func printHealth(app string, data map[string]interface{}) {
	output.Header(fmt.Sprintf("Healthcheck: %s", app))
	if enabled, _ := data["enabled"].(bool); !enabled {
		output.Warn("Healthcheck not configured — use 'cipi-cli health set %s'", app)
		fmt.Println()
		return
	}
	output.KeyValue(nil, "URL", str(data, "url"))
	output.KeyValue(nil, "Expect", str(data, "expect"))
	output.KeyValue(nil, "State", colorStatus(str(data, "state")))
	output.KeyValue(nil, "Failures", str(data, "failcount"))
	fmt.Println()
}

func init() {
	healthSetCmd.Flags().String("url", "", "URL to check (default: https://<primary domain>/up)")
	healthSetCmd.Flags().Int("expect", 200, "Expected HTTP status code")

	healthCmd.AddCommand(healthListCmd, healthShowCmd, healthSetCmd, healthUnsetCmd, healthCheckCmd)
	rootCmd.AddCommand(healthCmd)
}
