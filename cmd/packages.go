package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Installing and removing optional packages from the API came with Cipi
// 5.14.0 and cipi/api 1.44 (ability packages-manage); listing them is older.
const packagesManageMinAPI = "1.44.0"

// hostPackage is one entry of the server's closed catalog, as
// `cipi package list --json` describes it. kind, binaries and missing came
// with Cipi 5.14.0: an older server leaves them empty.
type hostPackage struct {
	ID          string   `json:"id"`
	Packages    []string `json:"packages"`
	Description string   `json:"description"`
	Kind        string   `json:"kind"`
	Binaries    []string `json:"binaries"`
	Missing     []string `json:"missing"`
	Installed   bool     `json:"installed"`
	Partial     bool     `json:"partial"`
}

func (p hostPackage) status() string {
	switch {
	case p.Installed:
		return "installed"
	case p.Partial:
		return "partial"
	default:
		return "not installed"
	}
}

// provides is what the entry brings: its commands, or "libraries" for an
// entry with none (browsershot, fonts-cjk).
func (p hostPackage) provides() string {
	if len(p.Binaries) > 0 {
		return strings.Join(p.Binaries, ", ")
	}
	if p.Kind == "libraries" {
		return "libraries"
	}
	return "—"
}

// decodeHostPackages reads the data of GET /api/packages: {"packages": [...]}
// since Cipi 5.2.2, a bare list from the first API releases.
func decodeHostPackages(raw json.RawMessage) ([]hostPackage, error) {
	var wrapped struct {
		Packages []hostPackage `json:"packages"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Packages != nil {
		return wrapped.Packages, nil
	}
	var list []hostPackage
	if err := json.Unmarshal(raw, &list); err != nil {
		return nil, fmt.Errorf("unexpected packages answer: %w", err)
	}
	return list, nil
}

// A catalog id as Cipi writes them. The server checks the id against its own
// catalog whatever is sent; this only spares a round trip for a typo.
var packageIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,39}$`)

func validatePackageID(id string) error {
	if !packageIDPattern.MatchString(id) {
		return fmt.Errorf("'%s' is not a package id (lowercase letters, digits and dashes — see: cipi-cli packages)", id)
	}
	return nil
}

var packagesCmd = &cobra.Command{
	Use:     "packages",
	Aliases: []string{"package"},
	Short:   "Optional host packages: list, install, remove",
	Long: `The optional packages of the server: a closed catalog of tools and
libraries some Laravel packages need on the host, from Ubuntu's own
repositories — image optimisers, AVIF, ffmpeg, ImageMagick, poppler-utils,
the libraries for Browsershot's Chrome, CJK fonts, pdftk, headless LibreOffice,
pandoc, Tesseract OCR (Cipi 5.14.0).

Without a subcommand it lists the catalog (packages-view). Install and
remove need Cipi 5.14.0+, cipi/api 1.44+ and the packages-manage ability;
on an older server run 'cipi package install <id>' on the host.

  cipi-cli packages
  cipi-cli packages install browsershot
  cipi-cli packages remove ffmpeg -y`,
	Example: `  cipi-cli packages
  cipi-cli prod packages install libreoffice
  cipi-cli packages remove tesseract -y`,
	Args: cobra.NoArgs,
	RunE: runPackagesList,
}

var packagesListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "List the catalog and what is installed",
	Long: `List the server's optional packages: installed, partial (some apt packages
missing) or not installed, and the commands each one brings.

Requires the packages-view ability.`,
	Example: `  cipi-cli packages list
  cipi-cli packages list --json`,
	Args: cobra.NoArgs,
	RunE: runPackagesList,
}

func runPackagesList(cmd *cobra.Command, args []string) error {
	client, err := mustClient()
	if err != nil {
		return err
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := client.Get("/api/packages", &result); err != nil {
		output.Error("Failed to list packages: %s", apiErrorHint(err, "5.2.2", "packages-view"))
		return err
	}

	if jsonFlag {
		output.PrintJSON(result)
		return nil
	}

	pkgs, err := decodeHostPackages(result.Data)
	if err != nil {
		output.Error("%s", err)
		return err
	}

	output.Header("Optional packages")
	if len(pkgs) == 0 {
		fmt.Println("  The server lists no optional packages.")
		fmt.Println()
		return nil
	}

	table := output.NewTable("PACKAGE", "STATUS", "PROVIDES", "DESCRIPTION")
	for _, p := range pkgs {
		status := p.status()
		if p.Partial && !p.Installed && len(p.Missing) > 0 {
			status = fmt.Sprintf("partial (missing %s)", strings.Join(p.Missing, " "))
		}
		table.Row(p.ID, colorStatus(status), p.provides(), p.Description)
	}
	table.Flush()
	output.Dim.Println("  cipi-cli packages install <id>   /   cipi-cli packages remove <id>")
	fmt.Println()
	return nil
}

var packagesInstallCmd = &cobra.Command{
	Use:     "install <id>",
	Aliases: []string{"add"},
	Short:   "Install an entry of the catalog (async job)",
	Long: `Install one entry of the server's catalog ("cipi package install <id>" on the
host), or complete a partial one. apt takes from seconds to a few minutes.

Only ids of the server's own catalog are accepted: it is closed. A single apt
package of a group is not an id here ('webp' is part of image-optimizers).
For Browsershot the entry is 'browsershot': the libraries and fonts Chrome
needs — Chrome itself comes from Puppeteer in the app (npm install puppeteer).

Requires Cipi 5.14.0+, cipi/api 1.44+ and the packages-manage ability.

  cipi-cli packages install browsershot
  cipi-cli prod packages install libreoffice`,
	Example: `  cipi-cli packages install ffmpeg`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := validatePackageID(id); err != nil {
			output.Error("%s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Installing %s...", id)
		job, err := client.DoAsyncAndWaitResult("POST", "/api/packages/"+url.PathEscape(id), nil)
		if err != nil {
			output.Error("Package install failed: %s", featureErrorHint(err, packagesManageMinAPI, "packages-manage"))
			return err
		}

		if res, ok := jobResultMap(job); ok {
			if missing := stringList(res["missing"]); len(missing) > 0 {
				output.Warn("%s installed, but these are missing: %s", id, strings.Join(missing, " "))
				fmt.Println()
				return nil
			}
			if already, _ := res["already"].(bool); already {
				output.Success("%s is already installed", id)
				fmt.Println()
				return nil
			}
		}

		output.Success("%s installed", id)
		fmt.Println()
		return nil
	},
}

var packagesRemoveCmd = &cobra.Command{
	Use:     "remove <id>",
	Aliases: []string{"rm", "uninstall"},
	Short:   "Remove an entry of the catalog (async job)",
	Long: `Remove one entry of the server's catalog ("cipi package remove <id>" on the
host). It takes only what no other package still needs: for a library entry
(browsershot, fonts-cjk) what other software uses stays installed. Apps that
use the entry stop working. Prompts for confirmation unless -y / --yes is
passed.

Requires Cipi 5.14.0+, cipi/api 1.44+ and the packages-manage ability.

  cipi-cli packages remove ffmpeg
  cipi-cli packages remove ffmpeg -y`,
	Example: `  cipi-cli packages remove pandoc
  cipi-cli prod packages remove tesseract -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if err := validatePackageID(id); err != nil {
			output.Error("%s", err)
			return err
		}

		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Remove %s from the server? Apps that use it stop working.", id)) {
			output.Warn("Aborted")
			return nil
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Removing %s...", id)
		job, err := client.DoAsyncAndWaitResult("DELETE", "/api/packages/"+url.PathEscape(id), nil)
		if err != nil {
			output.Error("Package remove failed: %s", featureErrorHint(err, packagesManageMinAPI, "packages-manage"))
			return err
		}

		if res, ok := jobResultMap(job); ok {
			if removed, isBool := res["removed"].(bool); isBool && !removed {
				if still, _ := res["still_needed"].(bool); still {
					output.Warn("%s: other packages still use these libraries — nothing removed", id)
				} else {
					output.Warn("%s is not installed", id)
				}
				fmt.Println()
				return nil
			}
		}

		output.Success("%s removed", id)
		fmt.Println()
		return nil
	},
}

// stringList reads a JSON array of strings decoded into interface{}.
func stringList(v interface{}) []string {
	list, ok := v.([]interface{})
	if !ok {
		return nil
	}
	out := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func init() {
	packagesRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	packagesCmd.AddCommand(packagesListCmd, packagesInstallCmd, packagesRemoveCmd)
	rootCmd.AddCommand(packagesCmd)
}
