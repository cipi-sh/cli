package cmd

import (
	"fmt"
	"net/url"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var deployConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Show or update structured deploy.php options",
	Long: `Read or update deploy recipe options (keep_releases, hooks, node_build, etc.).

Requires Cipi 5.0.3+ and the apps-deploy-config token ability.

  cipi-cli deploy config show myapp
  cipi-cli deploy config set myapp --keep-releases 5 --migrate`,
}

var deployConfigShowCmd = &cobra.Command{
	Use:   "show <app>",
	Short: "Show deploy config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/deploy-config"), &result); err != nil {
			output.Error("Failed to read deploy config: %s", apiErrorHint(err, "5.0.3", "apps-deploy-config"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Deploy config: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var deployConfigSetCmd = &cobra.Command{
	Use:   "set <app>",
	Short: "Update deploy config",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := map[string]interface{}{}
		changed := false

		if cmd.Flags().Changed("keep-releases") {
			v, _ := cmd.Flags().GetInt("keep-releases")
			body["keep_releases"] = v
			changed = true
		}
		for _, name := range []string{"migrate", "optimize", "storage-link", "queue-restart", "horizon-terminate", "predeploy-snapshot"} {
			flag := cmd.Flags().Lookup(name)
			if flag != nil && flag.Changed {
				v, _ := cmd.Flags().GetBool(name)
				key := snakeFlag(name)
				body[key] = v
				changed = true
			}
		}
		if cmd.Flags().Changed("node-build") {
			v, _ := cmd.Flags().GetString("node-build")
			body["node_build"] = v
			changed = true
		}
		if extra, _ := cmd.Flags().GetStringArray("extra-artisan"); len(extra) > 0 {
			body["extra_artisan"] = extra
			changed = true
		}

		if !changed {
			output.Error("Pass at least one flag to update")
			return fmt.Errorf("no deploy config fields specified")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put(appAPIPath(args[0], "/deploy-config"), body, &result); err != nil {
			output.Error("Failed to update deploy config: %s", apiErrorHint(err, "5.0.3", "apps-deploy-config"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Deploy config updated for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var deployAuditCmd = &cobra.Command{
	Use:   "audit <app>",
	Short: "Show deploy audit ledger records",
	Long: `Read the hash-chained deploy audit ledger for an application.

Requires Cipi 5.4.0+ and deploy-manage ability.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		days, _ := cmd.Flags().GetInt("days")

		client, err := mustClient()
		if err != nil {
			return err
		}

		query := url.Values{}
		if days > 0 {
			query.Set("days", fmt.Sprintf("%d", days))
		}
		path := appAPIPath(args[0], "/deploy/audit")
		if q := query.Encode(); q != "" {
			path += "?" + q
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read deploy audit: %s", apiErrorHint(err, "5.4.0", "deploy-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Deploy audit: %s", args[0]))
		switch v := result.Data.(type) {
		case []interface{}:
			rows := make([]map[string]interface{}, 0, len(v))
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					rows = append(rows, m)
				}
			}
			if len(rows) == 0 {
				output.Warn("No audit records yet")
			} else {
				printMapSlice(rows)
			}
		default:
			printDataWrapper(result.Data)
		}
		return nil
	},
}

func snakeFlag(name string) string {
	out := make([]byte, 0, len(name))
	for i := 0; i < len(name); i++ {
		if name[i] == '-' {
			out = append(out, '_')
		} else {
			out = append(out, name[i])
		}
	}
	return string(out)
}

func init() {
	deployConfigSetCmd.Flags().Int("keep-releases", 0, "Number of releases to keep")
	deployConfigSetCmd.Flags().Bool("migrate", false, "Run migrations on deploy")
	deployConfigSetCmd.Flags().Bool("optimize", false, "Run optimize on deploy")
	deployConfigSetCmd.Flags().Bool("storage-link", false, "Run storage:link on deploy")
	deployConfigSetCmd.Flags().Bool("queue-restart", false, "Restart queue workers on deploy")
	deployConfigSetCmd.Flags().Bool("horizon-terminate", false, "Terminate Horizon on deploy")
	deployConfigSetCmd.Flags().Bool("predeploy-snapshot", false, "Snapshot before deploy")
	deployConfigSetCmd.Flags().String("node-build", "", "Node build command")
	deployConfigSetCmd.Flags().StringArray("extra-artisan", nil, "Extra artisan commands (repeatable)")

	deployConfigCmd.AddCommand(deployConfigShowCmd, deployConfigSetCmd)
	deployCmd.AddCommand(deployConfigCmd, deployAuditCmd)

	deployAuditCmd.Flags().Int("days", 0, "Limit to recent N days")
}
