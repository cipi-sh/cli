package cmd

import (
	"fmt"
	"net/url"
	"regexp"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// nodeRuntimesMinAPI is the cipi/api release that manages Node runtimes; the
// server also needs Cipi 5.9.0 (it answers 503 and says so on an older core).
const nodeRuntimesMinAPI = "1.39.0"

var nodeMajorPattern = regexp.MustCompile(`^[2-9][0-9]$`)

// validateNodeMajor rejects what the API would refuse: a Node major is an
// even (LTS) two-digit number. "system" — the NodeSource package — is only
// accepted where the API accepts it (the server default).
func validateNodeMajor(major string, allowSystem bool) error {
	if allowSystem && major == "system" {
		return nil
	}
	if nodeMajorPattern.MatchString(major) && (major[1]-'0')%2 == 0 {
		return nil
	}
	if allowSystem {
		return fmt.Errorf("invalid Node version %q — use an even (LTS) major, e.g. 22 or 24, or system", major)
	}
	return fmt.Errorf("invalid Node version %q — use an even (LTS) major, e.g. 22 or 24", major)
}

var nodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node runtimes and app Node status",
	Long: `Inspect and manage the Node runtimes of the server, or manage Node apps.

  cipi-cli node list                     server runtimes
  cipi-cli node install 24               install a Node major
  cipi-cli node upgrade [24] [--restart] latest patch of one major, or of all
  cipi-cli node default 24|system        the server-wide Node
  cipi-cli node remove 22                remove a Node major
  cipi-cli apps node status myapp        app Node config
  cipi-cli apps node restart myapp       blue/green restart (SSR)

list and the apps node commands require Cipi 5.4.0+ and the node-view /
node-manage abilities. install, upgrade, default and remove require Cipi
5.9.0+, cipi/api 1.39+ and the node-runtimes-manage ability — existing tokens
do not get it.

` + multiServerTip,
	Example: `  cipi-cli node list
  cipi-cli node install 24
  cipi-cli node upgrade --restart
  cipi-cli prod node default 24
  cipi-cli node remove 22 -y`,
}

var nodeListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"runtimes"},
	Short:   "List installed Node runtimes",
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data interface{} `json:"data"`
		}
		if err := client.Get("/api/node", &result); err != nil {
			output.Error("Failed to list Node runtimes: %s", apiErrorHint(err, "5.4.0", "node-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Node runtimes")
		printDataWrapper(result.Data)
		return nil
	},
}

var nodeInstallCmd = &cobra.Command{
	Use:   "install <major>",
	Short: "Install a Node major (async job)",
	Long: `Install a Node major next to the ones already there: the official build
from nodejs.org, checksum-verified ("cipi node install" on the host).

The major is an even (LTS) number, e.g. 22 or 24.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the node-runtimes-manage ability.

  cipi-cli node install 24
  cipi-cli prod node install 22`,
	Example: `  cipi-cli node install 24
  cipi-cli prod node install 22`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		major := args[0]
		if err := validateNodeMajor(major, false); err != nil {
			output.Error("%s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Installing Node %s...", major)
		if err := client.DoAsyncAndWait("POST", "/api/node/install", map[string]string{"major": major}); err != nil {
			output.Error("Node install failed: %s", featureErrorHint(err, nodeRuntimesMinAPI, "node-runtimes-manage"))
			return err
		}

		output.Success("Node %s installed", major)
		fmt.Println()
		return nil
	},
}

var nodeUpgradeCmd = &cobra.Command{
	Use:   "upgrade [major]",
	Short: "Upgrade one Node major, or every installed one, to its latest patch (async job)",
	Long: `Bring a Node major to its latest patch release — or every installed major
when none is given ("cipi node upgrade" on the host).

With --restart the SSR apps on the upgraded runtime move to the new binary
blue/green, with no downtime; without it they take it at their next deploy.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the node-runtimes-manage ability.

  cipi-cli node upgrade
  cipi-cli node upgrade 24 --restart`,
	Example: `  cipi-cli node upgrade
  cipi-cli node upgrade 24
  cipi-cli prod node upgrade --restart`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		restart, _ := cmd.Flags().GetBool("restart")
		body := map[string]interface{}{}
		what := "every installed Node major"
		if len(args) == 1 {
			if err := validateNodeMajor(args[0], false); err != nil {
				output.Error("%s", err)
				return err
			}
			body["major"] = args[0]
			what = "Node " + args[0]
		}
		if restart {
			body["restart"] = true
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Upgrading %s...", what)
		job, err := client.DoAsyncAndWaitResult("POST", "/api/node/upgrade", body)
		if err != nil {
			output.Error("Node upgrade failed: %s", featureErrorHint(err, nodeRuntimesMinAPI, "node-runtimes-manage"))
			return err
		}

		// The job has no parsed result: what was upgraded is in the CLI output.
		if job != nil {
			printCLIOutput(job.Output)
		}
		output.Success("Upgrade of %s finished", what)
		if !restart {
			output.Dim.Println("  SSR apps take the new binary at their next deploy (or pass --restart).")
		}
		fmt.Println()
		return nil
	},
}

var nodeDefaultCmd = &cobra.Command{
	Use:   "default <major|system>",
	Short: "Set the server-wide Node (async job)",
	Long: `Set the server-wide Node: what Laravel asset builds and "apps run npm" use
("cipi node default" on the host). Node apps and apps pinned to a major keep
theirs. A major that is not installed is installed first; "system" is the
NodeSource package.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the node-runtimes-manage ability.

  cipi-cli node default 24
  cipi-cli node default system`,
	Example: `  cipi-cli node default 24
  cipi-cli prod node default system`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		major := args[0]
		if err := validateNodeMajor(major, true); err != nil {
			output.Error("%s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Setting the server default Node to %s...", major)
		if err := client.DoAsyncAndWait("PUT", "/api/node/default", map[string]string{"major": major}); err != nil {
			output.Error("Failed to set the default Node: %s", featureErrorHint(err, nodeRuntimesMinAPI, "node-runtimes-manage"))
			return err
		}

		output.Success("Server default Node is now %s", major)
		fmt.Println()
		return nil
	},
}

var nodeRemoveCmd = &cobra.Command{
	Use:     "remove <major>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove a Node major (async job)",
	Long: `Remove an installed Node major ("cipi node remove" on the host).

The server refuses to remove a major an app uses or that is the server
default: the job then fails with its message. Prompts for confirmation unless
-y / --yes is passed.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the node-runtimes-manage ability.

  cipi-cli node remove 22
  cipi-cli node remove 22 -y`,
	Example: `  cipi-cli node remove 22
  cipi-cli prod node remove 22 -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		major := args[0]
		if err := validateNodeMajor(major, false); err != nil {
			output.Error("%s", err)
			return err
		}

		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Remove Node %s from the server?", major)) {
			output.Warn("Aborted")
			return nil
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Removing Node %s...", major)
		if err := client.DoAsyncAndWait("DELETE", "/api/node/"+url.PathEscape(major), nil); err != nil {
			output.Error("Node remove failed: %s", featureErrorHint(err, nodeRuntimesMinAPI, "node-runtimes-manage"))
			return err
		}

		output.Success("Node %s removed", major)
		fmt.Println()
		return nil
	},
}

var appsNodeCmd = &cobra.Command{
	Use:   "node",
	Short: "Node status and restart for an app",
}

var appsNodeStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show Node status for an app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/node"), &result); err != nil {
			output.Error("Failed to read Node status: %s", apiErrorHint(err, "5.4.0", "node-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Node: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var appsNodeRestartCmd = &cobra.Command{
	Use:   "restart <name>",
	Short: "Blue/green restart an SSR Node app",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Restarting Node app '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", appAPIPath(args[0], "/node/restart"), nil); err != nil {
			output.Error("Node restart failed: %s", apiErrorHint(err, "5.4.0", "node-manage"))
			return err
		}

		output.Success("Node app '%s' restarted", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsNodeCmd.AddCommand(appsNodeStatusCmd, appsNodeRestartCmd)
	appsCmd.AddCommand(appsNodeCmd)

	nodeUpgradeCmd.Flags().Bool("restart", false, "Restart SSR apps on the new binary (blue/green, no downtime)")
	nodeRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	nodeCmd.AddCommand(nodeListCmd, nodeInstallCmd, nodeUpgradeCmd, nodeDefaultCmd, nodeRemoveCmd)
	rootCmd.AddCommand(nodeCmd)
}
