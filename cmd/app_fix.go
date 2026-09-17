package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsFixPermissionsCmd = &cobra.Command{
	Use:   "fix-permissions <name>",
	Short: "Restore the app home permission model",
	Long: `Fix ownership and permissions under the app home directory.

Dispatched as an async job. Requires Cipi 5.2.1+ and apps-edit ability.

  cipi-cli apps fix-permissions myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Fixing permissions for '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", appAPIPath(args[0], "/fix-permissions"), nil); err != nil {
			output.Error("Failed to fix permissions: %s", apiErrorHint(err, "5.2.1", "apps-edit"))
			return err
		}

		output.Success("Permissions fixed for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsCmd.AddCommand(appsFixPermissionsCmd)
}
