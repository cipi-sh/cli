package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var sslCmd = &cobra.Command{
	Use:   "ssl",
	Short: "Manage SSL certificates",
	Long: `Install and manage Let's Encrypt SSL certificates for applications.

  cipi-cli ssl install myapp
  cipi-cli ssl force myapp
  cipi-cli prod ssl install myapp

DNS for the app domain (and aliases) must already point to the server.

` + multiServerTip,
	Example: `  cipi-cli ssl install myapp
  cipi-cli ssl force myapp
  cipi-cli prod ssl install myapp`,
}

var sslForceCmd = &cobra.Command{
	Use:   "force <app>",
	Short: "Re-apply HTTP → HTTPS redirect",
	Long: `Re-apply the HTTP to HTTPS redirect without issuing a new certificate.

Requires Cipi 4.8+.

  cipi-cli ssl force myapp
  cipi-cli prod ssl force myapp`,
	Example: `  cipi-cli ssl force myapp
  cipi-cli prod ssl force myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Forcing HTTPS redirect for '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/ssl/force", args[0]), nil); err != nil {
			output.Error("SSL force failed: %s", apiErrorHint(err, "4.8", "ssl-manage"))
			return err
		}

		output.Success("HTTPS redirect re-applied for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var sslInstallCmd = &cobra.Command{
	Use:   "install <app>",
	Short: "Install a Let's Encrypt SSL certificate",
	Long: `Request and install a Let's Encrypt certificate for the application's
primary domain and aliases.

Waits for the async job to finish before returning.

  cipi-cli ssl install myapp
  cipi-cli staging ssl install myapp`,
	Example: `  cipi-cli ssl install myapp
  cipi-cli ssl force myapp
  cipi-cli prod ssl install myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Installing SSL for '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/ssl", args[0]), nil); err != nil {
			output.Error("SSL installation failed: %s", err)
			return err
		}

		output.Success("SSL certificate installed for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	sslCmd.AddCommand(sslInstallCmd, sslForceCmd)
	rootCmd.AddCommand(sslCmd)
}
