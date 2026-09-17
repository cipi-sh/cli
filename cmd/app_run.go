package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var runCommandsCmd = &cobra.Command{
	Use:   "run-commands",
	Short: "List whitelisted app run commands",
	Long: `List binaries allowed by POST /api/apps/{name}/run.

Requires Cipi 5.0.3+ and the apps-run token ability.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get("/api/run-commands", &result); err != nil {
			output.Error("Failed to list run commands: %s", apiErrorHint(err, "5.0.3", "apps-run"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Whitelisted run commands")
		switch v := result.Data.(type) {
		case []interface{}:
			for _, item := range v {
				fmt.Printf("  • %v\n", item)
			}
		default:
			printDataWrapper(result.Data)
		}
		fmt.Println()
		return nil
	},
}

var appsRunCmd = &cobra.Command{
	Use:   "run <name> [command...]",
	Short: "Run a whitelisted command as the app user (async job)",
	Long: `Execute a non-interactive whitelisted binary as the app user.

Use 'cipi-cli run-commands' to see allowed commands. Requires Cipi 5.0.3+
and the apps-run token ability.

  cipi-cli apps run myapp composer install --no-dev
  cipi-cli apps run myapp npm run build`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args[1:], " ")

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"command": command}
		output.Info("Running on '%s': %s", args[0], command)
		if err := client.DoAsyncAndWait("POST", appAPIPath(args[0], "/run"), body); err != nil {
			output.Error("Run failed: %s", apiErrorHint(err, "5.0.3", "apps-run"))
			return err
		}

		output.Success("Command completed on '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsCmd.AddCommand(appsRunCmd)
	rootCmd.AddCommand(runCommandsCmd)
}
