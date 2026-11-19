package cmd

import (
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var phpCmd = &cobra.Command{
	Use:   "php",
	Short: "List and install PHP versions",
	Long: `Inspect PHP versions on the selected server or install a new one.

Requires Cipi 5.0.6+ (API 1.15+) and php-view / php-manage abilities.
Switching the default version and removing versions are host-only
('cipi php switch|remove' over SSH).

  cipi-cli php list
  cipi-cli php install 8.5

` + multiServerTip,
	Example: `  cipi-cli php list
  cipi-cli prod php list --json
  cipi-cli php install 8.5`,
}

var phpListCmd = &cobra.Command{
	Use:   "list",
	Short: "List installed PHP versions and the server default",
	Long: `List installed PHP versions with FPM status, apps per version, and the
server default, plus the versions that can be installed.

A server installed before Cipi 4.5.4 can still have PHP 7.4-8.2. They are
listed and marked legacy: the apps on them keep running and can be moved to
a current version ('apps edit <app> --php 8.5'), no app can be created on or
moved to one, and removing one is host-only ('cipi php remove').

  cipi-cli php list
  cipi-cli prod php list`,
	Example: `  cipi-cli php list
  cipi-cli php list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get("/api/php", &result); err != nil {
			output.Error("Failed to list PHP versions: %s", apiErrorHint(err, "1.15.0", "php-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		installable := []string{}
		if list, ok := result.Data["installable"].([]interface{}); ok {
			for _, v := range list {
				installable = append(installable, fmt.Sprintf("%v", v))
			}
		}

		output.Header("PHP versions")
		versions, _ := result.Data["versions"].([]interface{})
		legacy := []string{}
		if len(versions) == 0 {
			output.Warn("No PHP versions installed")
		} else {
			t := output.NewTable("VERSION", "STATUS", "APPS", "DEFAULT")
			for _, item := range versions {
				v, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				def := ""
				if b, _ := v["default"].(bool); b {
					def = output.Green.Sprint("✓")
				}
				version := str(v, "version")
				if isLegacyPHP(version, installable) {
					legacy = append(legacy, version)
					version += " (legacy)"
				}
				t.Row(version, colorStatus(str(v, "status")), str(v, "apps"), def)
			}
			t.Flush()
		}

		output.KeyValue(nil, "Default", str(result.Data, "default"))
		if len(installable) > 0 {
			output.KeyValue(nil, "Installable", strings.Join(installable, ", "))
		}
		if len(legacy) > 0 {
			output.KeyValue(nil, "Legacy", strings.Join(legacy, ", ")+" — apps on them keep running; no app can be created on or moved to one")
		}
		fmt.Println()
		return nil
	},
}

var phpInstallCmd = &cobra.Command{
	Use:   "install <version>",
	Short: "Install a PHP version (async job)",
	Long: `Install a PHP version with FPM and the standard extensions.

Allowed versions are 8.3, 8.4, 8.5, and 8.6 (8.6 needs Cipi 5.9.1+ and
API 1.39.2+ on the server). The job can take a few minutes.

  cipi-cli php install 8.5
  cipi-cli prod php install 8.4`,
	Example: `  cipi-cli php install 8.5`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Installing PHP %s...", args[0])
		body := map[string]string{"version": args[0]}
		if err := client.DoAsyncAndWait("POST", "/api/php/install", body); err != nil {
			output.Error("PHP install failed: %s", apiErrorHint(err, "1.15.0", "php-manage"))
			return err
		}

		output.Success("PHP %s installed", args[0])
		fmt.Println()
		return nil
	},
}

// isLegacyPHP reports whether an installed version is one Cipi no longer
// installs or gives to an app: older than everything the server lists as
// installable (7.4-8.2 on a server set up before Cipi 4.5.4). Without that
// list nothing is called legacy.
func isLegacyPHP(version string, installable []string) bool {
	if parseVersion(version) == nil || len(installable) == 0 {
		return false
	}
	for _, v := range installable {
		if parseVersion(v) != nil && compareVersions(version, v) >= 0 {
			return false
		}
	}
	return true
}

func init() {
	phpCmd.AddCommand(phpListCmd, phpInstallCmd)
	rootCmd.AddCommand(phpCmd)
}
