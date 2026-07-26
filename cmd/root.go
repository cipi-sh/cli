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
	Long: `Manage Cipi servers, apps, databases, SSL, and deployments from the command line.

` + multiServerTip + `

Quick start:
  cipi-cli configure --profile prod     add a server
  cipi-cli profiles                     list servers
  cipi-cli profiles use prod            set default server
  cipi-cli profiles delete staging      remove a server
  cipi-cli status                       overview of all servers
  cipi-cli prod apps list               run against "prod"

Aliases: "servers" works like "profiles" (e.g. cipi-cli servers use prod).`,
	Example: `  cipi-cli configure --profile prod
  cipi-cli profiles add staging
  cipi-cli profiles use prod
  cipi-cli profiles delete staging
  cipi-cli status
  cipi-cli prod apps list
  cipi-cli staging deploy myapp
  cipi-cli apps list`,
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
	Short: "Print CLI version and build time",
	Long: `Print the installed cipi-cli version and build timestamp.

  cipi-cli version
  cipi-cli version --json`,
	Example: `  cipi-cli version
  cipi-cli version --json`,
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
