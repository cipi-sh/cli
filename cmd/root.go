package cmd

import (
	"fmt"
	"os"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	Version   = "dev"
	BuildTime = "unknown"
	jsonFlag  bool
	noColor   bool
)

var rootCmd = &cobra.Command{
	Use:   "cipi-cli",
	Short: "CLI for Cipi Server Panel",
	Long: `Manage your Cipi servers, apps, databases, SSL certificates, and deployments from the command line.

Multi-server (profiles):
  Each profile is one Cipi server (endpoint + token).
  Prefix any command with the profile name to target that server.

  cipi-cli configure --profile prod     add a server
  cipi-cli profiles                     list servers
  cipi-cli profiles use prod            set default server
  cipi-cli prod apps list               run against "prod"
  cipi-cli apps list                    run against the default

Aliases: "servers" works like "profiles" (e.g. cipi-cli servers use prod).`,
	Example: `  cipi-cli configure --profile prod
  cipi-cli profiles add staging
  cipi-cli profiles use prod
  cipi-cli prod apps list
  cipi-cli staging deploy myapp`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if noColor {
			color.NoColor = true
		}
		output.JSONOutput = jsonFlag
	},
	Run: func(cmd *cobra.Command, args []string) {
		output.Banner()
		output.Dim.Println("  Use 'cipi-cli --help' to see available commands.")
		output.Dim.Println("  Multi-server: 'cipi-cli profiles' — each profile is one server.")
		fmt.Println()
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Run: func(cmd *cobra.Command, args []string) {
		if jsonFlag {
			output.PrintJSON(map[string]string{
				"version": Version,
				"build":   BuildTime,
			})
			return
		}
		fmt.Println()
		output.KeyValue(nil, "Version", Version)
		output.KeyValue(nil, "Build", BuildTime)
		fmt.Println()
	},
}

func Execute() {
	stripProfileArg()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.PersistentFlags().BoolVar(&jsonFlag, "json", false, "Output in JSON format")
	rootCmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "Disable colored output")
	rootCmd.AddCommand(versionCmd)
}
