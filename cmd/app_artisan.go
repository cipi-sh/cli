package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsArtisanCmd = &cobra.Command{
	Use:   "artisan <name> [command...]",
	Short: "Run an Artisan command (async job)",
	Long: `Run a Laravel Artisan command as an async job.

Poll output with 'cipi-cli jobs show <id>' if needed. Requires Cipi 5.0.3+
and the apps-artisan token ability.

  cipi-cli apps artisan myapp migrate --force
  cipi-cli apps artisan myapp cache:clear`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		command := strings.Join(args[1:], " ")

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"command": command}
		output.Info("Running artisan on '%s': %s", args[0], command)
		if err := client.DoAsyncAndWait("POST", appAPIPath(args[0], "/artisan"), body); err != nil {
			output.Error("Artisan failed: %s", apiErrorHint(err, "5.0.3", "apps-artisan"))
			return err
		}

		output.Success("Artisan command completed on '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsCmd.AddCommand(appsArtisanCmd)
}
