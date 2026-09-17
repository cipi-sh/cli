package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node runtimes and app Node status",
	Long: `Inspect installed Node runtimes or manage Node apps.

  cipi-cli node list                     server runtimes
  cipi-cli apps node status myapp        app Node config
  cipi-cli apps node restart myapp       blue/green restart (SSR)

Requires Cipi 5.4.0+ and node-view / node-manage abilities.`,
}

var nodeListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"runtimes"},
	Short:   "List installed Node runtimes",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get("/api/node", &result); err != nil {
			output.Error("Failed to list Node runtimes: %s", apiErrorHint(err, "5.4.0", "node-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Node runtimes")
		printDataWrapper(result.Data)
		return nil
	},
}

var appsNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node status and restart for an app",
}

var appsNodeStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show Node status for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/node"), &result); err != nil {
			output.Error("Failed to read Node status: %s", apiErrorHint(err, "5.4.0", "node-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Node: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var appsNodeRestartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Blue/green restart an SSR Node app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Restarting Node app '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", appAPIPath(args[0], "/node/restart"), nil); err != nil {
			output.Error("Node restart failed: %s", apiErrorHint(err, "5.4.0", "node-manage"))
			return err
		}

		output.Success("Node app '%s' restarted", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsNodeCmd.AddCommand(appsNodeStatusCmd, appsNodeRestartCmd)
	appsCmd.AddCommand(appsNodeCmd)

	nodeCmd.AddCommand(nodeListCmd)
	rootCmd.AddCommand(nodeCmd)
}
