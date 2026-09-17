package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:   "search",
	Short: "Meilisearch status and app search settings",
	Long: `Inspect Meilisearch on the server or enable/disable Scout search per app.

Requires Cipi 5.2.2+ and search-view / search-manage abilities.

  cipi-cli search status
  cipi-cli apps search enable myapp
  cipi-cli apps search disable myapp`,
}

var searchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show Meilisearch status and search-enabled apps",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/search", &result); err != nil {
			output.Error("Failed to read search status: %s", apiErrorHint(err, "5.2.2", "search-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Search status")
		printDataWrapper(result.Data)
		return nil
	},
}

var appsSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Enable or disable Meilisearch for an app",
}

var appsSearchEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable Meilisearch/Scout for a Laravel app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/search/enable"), nil, nil); err != nil {
			output.Error("Failed to enable search: %s", apiErrorHint(err, "5.2.2", "search-manage"))
			return err
		}

		output.Success("Search enabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var appsSearchDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable search and restore previous SCOUT_DRIVER",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/search/disable"), nil, nil); err != nil {
			output.Error("Failed to disable search: %s", apiErrorHint(err, "5.2.2", "search-manage"))
			return err
		}

		output.Success("Search disabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	searchCmd.AddCommand(searchStatusCmd)
	rootCmd.AddCommand(searchCmd)

	appsSearchCmd.AddCommand(appsSearchEnableCmd, appsSearchDisableCmd)
	appsCmd.AddCommand(appsSearchCmd)
}
