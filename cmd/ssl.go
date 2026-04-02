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
}

var sslInstallCmd = &cobra.Command{
	Use:   "install [app]",
	Short: "Install a Let's Encrypt SSL certificate",
	Args:  cobra.ExactArgs(1),
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
	sslCmd.AddCommand(sslInstallCmd)
	rootCmd.AddCommand(sslCmd)
}
