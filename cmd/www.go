package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var wwwCmd = &cobra.Command{
	Use:   "www",
	Short: "Manage www/apex redirects for an app",
	Long: `Show or configure www ↔ apex redirect behavior.

Requires Cipi 4.8+ and the www-manage token ability.

  cipi-cli www status myapp
  cipi-cli www add myapp
  cipi-cli www force-to-root myapp
  cipi-cli www force-from-root myapp
  cipi-cli www clear myapp

` + multiServerTip,
}

var wwwStatusCmd = &cobra.Command{
	Use:   "status <app>",
	Short: "Show www/apex redirect status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/www"), &result); err != nil {
			output.Error("Failed to read www status: %s", apiErrorHint(err, "4.8", "www-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("WWW redirect: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

func runWWWAsyncAction(app, action, label string) error {
	client, err := mustClient()
	if err != nil {
		return err
	}

	output.Info("%s for '%s'...", label, app)
	if err := client.DoAsyncAndWait("POST", appAPIPath(app, "/www/"+action), nil); err != nil {
		output.Error("Failed: %s", apiErrorHint(err, "4.8", "www-manage"))
		return err
	}

	output.Success("%s completed for '%s'", label, app)
	fmt.Println()
	return nil
}

var wwwAddCmd = &cobra.Command{
	Use:   "add <app>",
	Short: "Add www/apex counterpart alias",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runWWWAsyncAction(args[0], "add", "Adding www alias") },
}

var wwwForceToRootCmd = &cobra.Command{
	Use:   "force-to-root <app>",
	Short: "301 redirect www → apex",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runWWWAsyncAction(args[0], "force-to-root", "Forcing www → apex") },
}

var wwwForceFromRootCmd = &cobra.Command{
	Use:   "force-from-root <app>",
	Short: "301 redirect apex → www",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runWWWAsyncAction(args[0], "force-from-root", "Forcing apex → www") },
}

var wwwClearCmd = &cobra.Command{
	Use:   "clear <app>",
	Short: "Clear www canonical redirect",
	Args:  cobra.ExactArgs(1),
	RunE:  func(cmd *cobra.Command, args []string) error { return runWWWAsyncAction(args[0], "clear", "Clearing www redirect") },
}

func init() {
	wwwCmd.AddCommand(wwwStatusCmd, wwwAddCmd, wwwForceToRootCmd, wwwForceFromRootCmd, wwwClearCmd)
	rootCmd.AddCommand(wwwCmd)
}
