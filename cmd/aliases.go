package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var aliasesCmd = &cobra.Command{
	Use:     "aliases",
	Aliases: []string{"alias"},
	Short:   "Manage application domain aliases",
	Long: `Manage extra domains (aliases) attached to an application.

The primary domain is set on the app; aliases are additional hostnames
that point to the same site.

  cipi-cli aliases list myapp
  cipi-cli aliases add myapp www.example.com
  cipi-cli aliases remove myapp www.example.com

See also: 'cipi-cli domains' for a flat list of every domain across apps.

` + multiServerTip,
	Example: `  cipi-cli aliases list myapp
  cipi-cli aliases add myapp www.example.com
  cipi-cli aliases remove myapp www.example.com -y
  cipi-cli prod aliases list myapp`,
}

var aliasesListCmd = &cobra.Command{
	Use:   "list <app>",
	Short: "List aliases for an application",
	Long: `List domain aliases for one application.

  cipi-cli aliases list myapp
  cipi-cli prod aliases list myapp`,
	Example: `  cipi-cli aliases list myapp
  cipi-cli staging aliases list myapp`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		var result struct {
			Data []map[string]interface{} `json:"data"`
		}

		if err := client.Get(fmt.Sprintf("/api/apps/%s/aliases", args[0]), &result); err != nil {
			output.Error("Failed to list aliases: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(result.Data) == 0 {
			output.Warn("No aliases for app '%s'", args[0])
			return nil
		}

		output.Header(fmt.Sprintf("Aliases for %s", args[0]))
		t := output.NewTable("DOMAIN")
		for _, alias := range result.Data {
			t.Row(str(alias, "domain"))
		}
		t.Flush()
		return nil
	},
}

var aliasesAddCmd = &cobra.Command{
	Use:   "add <app> <domain>",
	Short: "Add an alias to an application",
	Long: `Attach an extra domain (alias) to an application.

DNS for the domain must already point to the server.

  cipi-cli aliases add myapp www.example.com
  cipi-cli prod aliases add myapp www.example.com`,
	Example: `  cipi-cli aliases add myapp www.example.com
  cipi-cli staging aliases add myapp api.example.com`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		body := map[string]string{
			"domain": args[1],
		}

		output.Info("Adding alias '%s' to app '%s'...", args[1], args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/apps/%s/aliases", args[0]), body); err != nil {
			output.Error("Failed to add alias: %s", err)
			return err
		}

		output.Success("Alias '%s' added to '%s'", args[1], args[0])
		fmt.Println()
		return nil
	},
}

var aliasesRemoveCmd = &cobra.Command{
	Use:   "remove <app> <domain>",
	Short: "Remove an alias from an application",
	Long: `Remove a domain alias from an application.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli aliases remove myapp www.example.com
  cipi-cli aliases remove myapp www.example.com -y`,
	Example: `  cipi-cli aliases remove myapp www.example.com
  cipi-cli aliases remove myapp www.example.com -y`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Remove alias '%s' from '%s'?", args[1], args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		body := map[string]string{
			"domain": args[1],
		}

		output.Info("Removing alias '%s' from '%s'...", args[1], args[0])
		if err := client.DoAsyncAndWait("DELETE", fmt.Sprintf("/api/apps/%s/aliases", args[0]), body); err != nil {
			output.Error("Failed to remove alias: %s", err)
			return err
		}

		output.Success("Alias '%s' removed from '%s'", args[1], args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	aliasesRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	aliasesCmd.AddCommand(aliasesListCmd, aliasesAddCmd, aliasesRemoveCmd)
	rootCmd.AddCommand(aliasesCmd)
}
