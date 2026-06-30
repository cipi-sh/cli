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
	Long:  "Manage your Cipi servers, apps, databases, SSL certificates, and deployments from the command line.\n\nUse a profile prefix to target a specific server: cipi-cli prod apps list",
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if noColor {
			color.NoColor = true
		}
		output.JSONOutput = jsonFlag
	},
	Run: func(cmd *cobra.Command, args []string) {
		output.Welcome()
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
		output.Header("Version")
		output.KeyValueDots(nil, "Version", Version)
		output.KeyValueDots(nil, "Build", BuildTime)
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
