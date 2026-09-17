package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var redirectCmd = &cobra.Command{
	Use:     "redirect",
	Aliases: []string{"redirects"},
	Short:   "Manage whole-app and path redirects",
	Long: `Configure app-wide or path-level redirects.

Requires Cipi 5.3.1+ (API sudoers 5.4.1+) and redirects-view / redirects-manage.

  cipi-cli redirect list myapp
  cipi-cli redirect set myapp --to https://new.example.com
  cipi-cli redirect add myapp --from /blog/ --to https://blog.example.com/

` + multiServerTip,
}

var redirectListCmd = &cobra.Command{
	Use:   "list <app>",
	Short: "List whole-app and path redirects",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/redirects"), &result); err != nil {
			output.Error("Failed to list redirects: %s", apiErrorHint(err, "5.4.1", "redirects-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Redirects: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var redirectSetCmd = &cobra.Command{
	Use:   "set <app>",
	Short: "Redirect every hostname of an app to a URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		to, _ := cmd.Flags().GetString("to")
		if to == "" {
			output.Error("--to is required")
			return fmt.Errorf("missing --to")
		}

		body := map[string]interface{}{"to": to}
		if cmd.Flags().Changed("code") {
			code, _ := cmd.Flags().GetInt("code")
			body["code"] = code
		}
		if cmd.Flags().Changed("keep-path") {
			keep, _ := cmd.Flags().GetBool("keep-path")
			body["keep_path"] = keep
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Put(appAPIPath(args[0], "/redirect"), body, nil); err != nil {
			output.Error("Failed to set redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Whole-app redirect set for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var redirectUnsetCmd = &cobra.Command{
	Use:   "unset <app>",
	Short: "Remove the whole-app redirect",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Delete(appAPIPath(args[0], "/redirect"), nil, nil); err != nil {
			output.Error("Failed to unset redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Whole-app redirect removed for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var redirectEnableCmd = &cobra.Command{
	Use:   "enable <app>",
	Short: "Enable the saved whole-app redirect",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/redirect/enable"), nil, nil); err != nil {
			output.Error("Failed to enable redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Whole-app redirect enabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var redirectDisableCmd = &cobra.Command{
	Use:   "disable <app>",
	Short: "Disable the saved whole-app redirect",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/redirect/disable"), nil, nil); err != nil {
			output.Error("Failed to disable redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Whole-app redirect disabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var redirectAddCmd = &cobra.Command{
	Use:   "add <app>",
	Short: "Add or update a path redirect",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		to, _ := cmd.Flags().GetString("to")
		if from == "" || to == "" {
			output.Error("--from and --to are required")
			return fmt.Errorf("missing --from or --to")
		}

		body := map[string]interface{}{"from": from, "to": to}
		if cmd.Flags().Changed("code") {
			code, _ := cmd.Flags().GetInt("code")
			body["code"] = code
		}
		if cmd.Flags().Changed("keep-path") {
			keep, _ := cmd.Flags().GetBool("keep-path")
			body["keep_path"] = keep
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/redirects"), body, nil); err != nil {
			output.Error("Failed to add path redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Path redirect added for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var redirectRemoveCmd = &cobra.Command{
	Use:   "remove <app>",
	Short: "Remove a path redirect",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		from, _ := cmd.Flags().GetString("from")
		if from == "" {
			output.Error("--from is required")
			return fmt.Errorf("missing --from")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"from": from}
		if err := client.Delete(appAPIPath(args[0], "/redirects"), body, nil); err != nil {
			output.Error("Failed to remove path redirect: %s", apiErrorHint(err, "5.4.1", "redirects-manage"))
			return err
		}

		output.Success("Path redirect removed for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	redirectSetCmd.Flags().String("to", "", "Target URL")
	redirectSetCmd.Flags().Int("code", 301, "HTTP redirect code (301, 302, 307, 308)")
	redirectSetCmd.Flags().Bool("keep-path", true, "Keep request path and query on the target")

	redirectAddCmd.Flags().String("from", "", "Source path (/blog/ = prefix match)")
	redirectAddCmd.Flags().String("to", "", "Target path or URL")
	redirectAddCmd.Flags().Int("code", 301, "HTTP redirect code")
	redirectAddCmd.Flags().Bool("keep-path", true, "Append remaining URI for prefix redirects")

	redirectRemoveCmd.Flags().String("from", "", "Source path to remove")

	redirectCmd.AddCommand(
		redirectListCmd, redirectSetCmd, redirectUnsetCmd,
		redirectEnableCmd, redirectDisableCmd, redirectAddCmd, redirectRemoveCmd,
	)
	rootCmd.AddCommand(redirectCmd)
}
