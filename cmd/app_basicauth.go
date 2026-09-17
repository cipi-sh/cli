package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsBasicAuthCmd = &cobra.Command{
	Use:     "basicauth",
	Aliases: []string{"basic-auth"},
	Short:   "Manage HTTP Basic Auth on an app",
	Long: `Enable, disable, or inspect Nginx HTTP Basic Auth for an application.

  cipi-cli apps basicauth status myapp
  cipi-cli apps basicauth enable myapp --user admin
  cipi-cli apps basicauth disable myapp`,
}

var appsBasicAuthStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show HTTP Basic Auth status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/basicauth"), &result); err != nil {
			output.Error("Failed to read basic auth status: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Basic Auth: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var appsBasicAuthEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable HTTP Basic Auth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")

		body := map[string]string{}
		if user != "" {
			body["user"] = user
		}
		if password != "" {
			body["password"] = password
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post(appAPIPath(args[0], "/basicauth/enable"), body, &result); err != nil {
			output.Error("Failed to enable basic auth: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("HTTP Basic Auth enabled for '%s'", args[0])
		if pwd, ok := result.Data["password"].(string); ok && pwd != "" {
			output.KeyValue(nil, "Password", pwd)
		}
		fmt.Println()
		return nil
	},
}

var appsBasicAuthDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable HTTP Basic Auth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/basicauth/disable"), nil, nil); err != nil {
			output.Error("Failed to disable basic auth: %s", err)
			return err
		}

		output.Success("HTTP Basic Auth disabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsBasicAuthEnableCmd.Flags().String("user", "", "Username (default: admin)")
	appsBasicAuthEnableCmd.Flags().String("password", "", "Password (auto-generated when omitted)")

	appsBasicAuthCmd.AddCommand(appsBasicAuthStatusCmd, appsBasicAuthEnableCmd, appsBasicAuthDisableCmd)
	appsCmd.AddCommand(appsBasicAuthCmd)
}
