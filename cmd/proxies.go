package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var proxiesCmd = &cobra.Command{
	Use:     "proxies",
	Aliases: []string{"proxy"},
	Short:   "Manage prefix reverse proxies for an app",
	Long: `List, add, or remove prefix reverse proxies.

Requires Cipi 5.3.1+ (API sudoers 5.4.1+) and proxies-view / proxies-manage.

  cipi-cli proxies list myapp
  cipi-cli proxies add myapp --prefix /api/ --upstream https://api.internal.example.com
  cipi-cli proxies remove myapp --prefix /api/

` + multiServerTip,
}

var proxiesListCmd = &cobra.Command{
	Use:   "list <app>",
	Short: "List prefix reverse proxies",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/proxies"), &result); err != nil {
			output.Error("Failed to list proxies: %s", apiErrorHint(err, "5.4.1", "proxies-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Proxies: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var proxiesAddCmd = &cobra.Command{
	Use:   "add <app>",
	Short: "Add or update a prefix proxy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		upstream, _ := cmd.Flags().GetString("upstream")
		if prefix == "" || upstream == "" {
			output.Error("--prefix and --upstream are required")
			return fmt.Errorf("missing --prefix or --upstream")
		}

		body := map[string]interface{}{
			"prefix":   prefix,
			"upstream": upstream,
		}
		if cmd.Flags().Changed("strip-prefix") {
			v, _ := cmd.Flags().GetBool("strip-prefix")
			body["strip_prefix"] = v
		}
		if cmd.Flags().Changed("preserve-host") {
			v, _ := cmd.Flags().GetBool("preserve-host")
			body["preserve_host"] = v
		}
		if cmd.Flags().Changed("timeout") {
			v, _ := cmd.Flags().GetInt("timeout")
			body["timeout"] = v
		}
		if cmd.Flags().Changed("buffering") {
			v, _ := cmd.Flags().GetBool("buffering")
			body["buffering"] = v
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/proxies"), body, nil); err != nil {
			output.Error("Failed to add proxy: %s", apiErrorHint(err, "5.4.1", "proxies-manage"))
			return err
		}

		output.Success("Proxy added for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var proxiesRemoveCmd = &cobra.Command{
	Use:   "remove <app>",
	Short: "Remove a prefix proxy",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		prefix, _ := cmd.Flags().GetString("prefix")
		if prefix == "" {
			output.Error("--prefix is required")
			return fmt.Errorf("missing --prefix")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"prefix": prefix}
		if err := client.Delete(appAPIPath(args[0], "/proxies"), body, nil); err != nil {
			output.Error("Failed to remove proxy: %s", apiErrorHint(err, "5.4.1", "proxies-manage"))
			return err
		}

		output.Success("Proxy removed for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	proxiesAddCmd.Flags().String("prefix", "", "Path prefix to proxy")
	proxiesAddCmd.Flags().String("upstream", "", "Upstream URL")
	proxiesAddCmd.Flags().Bool("strip-prefix", false, "Strip prefix before passing upstream")
	proxiesAddCmd.Flags().Bool("preserve-host", false, "Send original Host header")
	proxiesAddCmd.Flags().Int("timeout", 60, "Upstream timeout in seconds")
	proxiesAddCmd.Flags().Bool("buffering", true, "Enable response buffering")

	proxiesRemoveCmd.Flags().String("prefix", "", "Prefix to remove")

	proxiesCmd.AddCommand(proxiesListCmd, proxiesAddCmd, proxiesRemoveCmd)
	rootCmd.AddCommand(proxiesCmd)
}
