package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage Laravel app .env variables",
	Long: `Read or merge .env key/value pairs for a Laravel application.

Requires Cipi 5.0.3+ and the apps-env token ability.

  cipi-cli apps env show myapp
  cipi-cli apps env set myapp --set APP_DEBUG=false --unset OLD_KEY`,
}

var appsEnvShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show .env variables (secrets redacted)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/env"), &result); err != nil {
			output.Error("Failed to read env: %s", apiErrorHint(err, "5.0.3", "apps-env"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf(".env: %s", args[0]))
		if vars, ok := result.Data["variables"].(map[string]interface{}); ok {
			printMapFields(vars)
		} else {
			printDataWrapper(result.Data)
		}
		return nil
	},
}

var appsEnvSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Merge or unset .env keys",
	Long: `Update .env keys without replacing the whole file.

  cipi-cli apps env set myapp --set MAIL_HOST=smtp.example.com --unset OLD_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		setPairs, _ := cmd.Flags().GetStringArray("set")
		unsetKeys, _ := cmd.Flags().GetStringArray("unset")

		if len(setPairs) == 0 && len(unsetKeys) == 0 {
			output.Error("Pass at least one --set KEY=VALUE or --unset KEY")
			return fmt.Errorf("no env changes specified")
		}

		body := map[string]interface{}{}
		if len(setPairs) > 0 {
			setMap, err := parseKeyValuePairs(setPairs)
			if err != nil {
				output.Error("%s", err)
				return err
			}
			body["set"] = setMap
		}
		if len(unsetKeys) > 0 {
			body["unset"] = unsetKeys
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put(appAPIPath(args[0], "/env"), body, &result); err != nil {
			output.Error("Failed to update env: %s", apiErrorHint(err, "5.0.3", "apps-env"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Updated .env for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsEnvSetCmd.Flags().StringArray("set", nil, "Set KEY=VALUE (repeatable)")
	appsEnvSetCmd.Flags().StringArray("unset", nil, "Remove KEY (repeatable)")

	appsEnvCmd.AddCommand(appsEnvShowCmd, appsEnvSetCmd)
	appsCmd.AddCommand(appsEnvCmd)
}
