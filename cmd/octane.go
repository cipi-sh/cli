package cmd

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// The PHP of an Octane app by its number came with Cipi 5.17.0 and cipi/api
// 1.47 (abilities php-view, php-manage); the builds and their upgrade are
// Cipi 5.10.0 / cipi/api 1.40.
const (
	octaneMinAPI       = "1.47.0"
	octaneBuildsMinAPI = "1.40.0"
)

var octanePHPPattern = regexp.MustCompile(`^[0-9]\.[0-9]$`)

// octanePHP is the data of GET /api/apps/{app}/octane.
type octanePHP struct {
	App            string   `json:"app"`
	PHP            string   `json:"php"`
	OctanePHP      string   `json:"octane_php"`
	Build          string   `json:"build"`
	FrankenPHP     string   `json:"frankenphp"`
	State          string   `json:"state"`
	RestartPending bool     `json:"restart_pending"`
	AvailablePHP   []string `json:"available_php"`
	Fix            *struct {
		Command string `json:"command"`
		Why     string `json:"why"`
	} `json:"fix"`
}

func octanePath(app, suffix string) string {
	return "/api/apps/" + url.PathEscape(app) + "/octane" + suffix
}

// octaneFixHere turns the server's fix ("cipi frankenphp php shop 8.5") into
// the command of this client.
func octaneFixHere(command string) string {
	if rest, ok := strings.CutPrefix(command, "cipi frankenphp php "); ok {
		return "cipi-cli octane php " + rest
	}
	if rest, ok := strings.CutPrefix(command, "cipi php install "); ok {
		return "cipi-cli php install " + rest
	}
	return command
}

var octaneCmd = &cobra.Command{
	Use:   "octane",
	Short: "The PHP of an Octane app: show, switch by version, restart, builds",
	Long: `An Octane app runs two PHP. The one built into its FrankenPHP binary answers
the requests; the server's php<X.Y> runs Composer, artisan, cron and the queue
workers. A FrankenPHP release carries one PHP, so Cipi pins a build per app.

"octane php <app> <version>" puts both on one version by its number, as for
any other app: the build that carries it is activated (and fetched when the
server has none), Octane is restarted and checked — the previous build comes
back when it does not answer — then the rest of the app moves.

Requires Cipi 5.17.0+, cipi/api 1.47+ and the php-view / php-manage abilities.

  cipi-cli octane show shop
  cipi-cli octane php shop 8.5
  cipi-cli octane restart shop
  cipi-cli octane builds
  cipi-cli octane upgrade shop

` + multiServerTip,
}

var octaneShowCmd = &cobra.Command{
	Use:   "show <app>",
	Short: "Which PHP answers the requests, which runs the rest",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data octanePHP `json:"data"`
		}
		if err := client.Get(octanePath(args[0], ""), &result); err != nil {
			output.Error("Failed to read the PHP of %s: %s", args[0], featureErrorHint(err, octaneMinAPI, "php-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		o := result.Data
		output.Header("Octane — " + o.App)
		requests := o.OctanePHP
		if requests == "" {
			requests = "none yet"
		}
		if o.Build != "" {
			requests += "  (FrankenPHP build " + o.Build + ")"
		}
		output.KeyValue(nil, "Requests", "PHP "+requests)
		output.KeyValue(nil, "The rest", "PHP "+o.PHP+"  (Composer, artisan, cron, workers)")
		output.KeyValue(nil, "State", colorStatus(o.State))
		if o.RestartPending {
			output.KeyValue(nil, "Restart", "pending — cipi-cli octane restart "+o.App)
		}
		if len(o.AvailablePHP) > 0 {
			output.KeyValue(nil, "Builds here", "PHP "+strings.Join(o.AvailablePHP, ", ")+"  (another version is fetched when asked for)")
		}
		if o.Fix != nil && o.Fix.Command != "" {
			fmt.Println()
			output.Warn("Two PHP versions in one app: %s", octaneFixHere(o.Fix.Command))
			if o.Fix.Why != "" {
				fmt.Printf("    %s\n", o.Fix.Why)
			}
		}
		fmt.Println()
		return nil
	},
}

var octanePHPCmd = &cobra.Command{
	Use:   "php <app> <version>",
	Short: "Put an Octane app on a PHP version (async job)",
	Long: `Put the app on one PHP version: Octane and the server's PHP together.

The version has to be installed on the server (cipi-cli php install 8.5). A
FrankenPHP release carries one PHP: when none carries the version asked for,
the job fails and says which versions exist.

  cipi-cli octane php shop 8.5`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if !octanePHPPattern.MatchString(args[1]) {
			err := fmt.Errorf("invalid PHP version %q — a number such as 8.5", args[1])
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		output.Info("Putting %s on PHP %s...", args[0], args[1])
		job, err := client.DoAsyncAndWaitResult("PUT", octanePath(args[0], "/php"), map[string]string{"php": args[1]})
		if err != nil {
			if job != nil && job.Output != "" {
				printCLIOutput(job.Output)
			}
			output.Error("%s was not moved: %s", args[0], featureErrorHint(err, octaneMinAPI, "php-manage"))
			return err
		}
		if m, ok := jobResultMap(job); ok {
			if changed, _ := m["changed"].(bool); !changed {
				output.Info("%s is on PHP %s already", args[0], args[1])
				fmt.Println()
				return nil
			}
			output.Success("%s is on PHP %s — Octane on %s", args[0], args[1], str(m, "build"))
		} else {
			output.Success("%s is on PHP %s", args[0], args[1])
		}
		fmt.Println()
		return nil
	},
}

var octaneRestartCmd = &cobra.Command{
	Use:   "restart <app>",
	Short: "Restart Octane on its build and check that it answers (async job)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		output.Info("Restarting Octane of %s...", args[0])
		job, err := client.DoAsyncAndWaitResult("POST", octanePath(args[0], "/restart"), nil)
		if err != nil {
			if job != nil && job.Output != "" {
				printCLIOutput(job.Output)
			}
			output.Error("Restart failed: %s", featureErrorHint(err, octaneMinAPI, "php-manage"))
			return err
		}
		if m, ok := jobResultMap(job); ok {
			if stopped, _ := m["stopped"].(bool); stopped {
				output.Warn("Octane of %s is stopped — left as it is", args[0])
				fmt.Println()
				return nil
			}
		}
		output.Success("Octane of %s restarted and answering", args[0])
		fmt.Println()
		return nil
	},
}

var octaneBuildsCmd = &cobra.Command{
	Use:   "builds",
	Short: "The FrankenPHP build of every Octane app",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result struct {
			Data struct {
				Apps []map[string]interface{} `json:"apps"`
			} `json:"data"`
		}
		if err := client.Get("/api/frankenphp", &result); err != nil {
			output.Error("Failed to read the FrankenPHP builds: %s", featureErrorHint(err, octaneBuildsMinAPI, "php-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Header("FrankenPHP builds")
		if len(result.Data.Apps) == 0 {
			output.Warn("No Octane app")
			fmt.Println()
			return nil
		}
		t := output.NewTable("APP", "PHP", "OCTANE PHP", "FRANKENPHP", "STATE")
		for _, a := range result.Data.Apps {
			t.Row(str(a, "app"), str(a, "php"), str(a, "build_php"), str(a, "frankenphp"), colorStatus(str(a, "state")))
		}
		t.Flush()
		fmt.Println()
		return nil
	},
}

var octaneUpgradeCmd = &cobra.Command{
	Use:   "upgrade [app]",
	Short: "Newest FrankenPHP build on the app's own PHP (async job)",
	Long: `Move one Octane app — or all of them, without a name — to the newest
FrankenPHP build on its own PHP minor. Never another PHP version: that is
"octane php <app> <version>".

  cipi-cli octane upgrade shop
  cipi-cli octane upgrade`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var body interface{}
		if len(args) == 1 {
			body = map[string]string{"app": args[0]}
			output.Info("Upgrading the FrankenPHP build of %s...", args[0])
		} else {
			output.Info("Upgrading the FrankenPHP build of every Octane app...")
		}
		job, err := client.DoAsyncAndWaitResult("POST", "/api/frankenphp/upgrade", body)
		if job != nil && job.Output != "" {
			printCLIOutput(job.Output)
		}
		if err != nil {
			output.Error("Upgrade failed: %s", featureErrorHint(err, octaneBuildsMinAPI, "php-manage"))
			return err
		}
		output.Success("Done")
		fmt.Println()
		return nil
	},
}

func init() {
	octaneCmd.AddCommand(octaneShowCmd, octanePHPCmd, octaneRestartCmd, octaneBuildsCmd, octaneUpgradeCmd)
	rootCmd.AddCommand(octaneCmd)
}
