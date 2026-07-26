package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var deployCmd = &cobra.Command{
	Use:   "deploy <app>",
	Short: "Deploy an application",
	Long: `Trigger a deployment for an application (pull + release).

Waits for the async job to finish before returning.

  cipi-cli deploy myapp
  cipi-cli prod deploy myapp

Subcommands:
  cipi-cli deploy rollback myapp   previous release
  cipi-cli deploy unlock myapp     clear a stuck deploy lock

` + multiServerTip,
	Example: `  cipi-cli deploy myapp
  cipi-cli prod deploy myapp
  cipi-cli deploy rollback myapp
  cipi-cli deploy unlock myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Deploying '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/deploy", args[0]), nil); err != nil {
			output.Error("Deploy failed: %s", err)
			return err
		}

		output.Success("App '%s' deployed successfully", args[0])
		fmt.Println()
		return nil
	},
}

var deployRollbackCmd = &cobra.Command{
	Use:   "rollback <app>",
	Short: "Rollback to the previous release",
	Long: `Roll back an application to the previous release.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli deploy rollback myapp
  cipi-cli deploy rollback myapp -y`,
	Example: `  cipi-cli deploy rollback myapp
  cipi-cli prod deploy rollback myapp -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Rollback app '%s' to previous release?", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Rolling back '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/deploy/rollback", args[0]), nil); err != nil {
			output.Error("Rollback failed: %s", err)
			return err
		}

		output.Success("App '%s' rolled back successfully", args[0])
		fmt.Println()
		return nil
	},
}

var deployUnlockCmd = &cobra.Command{
	Use:   "unlock <app>",
	Short: "Unlock a stuck deployment",
	Long: `Clear a stuck deploy lock so a new deployment can run.

Use when a previous deploy crashed or was interrupted and left the
application locked.

  cipi-cli deploy unlock myapp
  cipi-cli prod deploy unlock myapp`,
	Example: `  cipi-cli deploy unlock myapp
  cipi-cli staging deploy unlock myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Unlocking deploy for '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/deploy/unlock", args[0]), nil); err != nil {
			output.Error("Unlock failed: %s", err)
			return err
		}

		output.Success("Deploy unlocked for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	deployRollbackCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	deployCmd.AddCommand(deployRollbackCmd, deployUnlockCmd)
	rootCmd.AddCommand(deployCmd)
}
