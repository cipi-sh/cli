package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// ymlRequires is what a server needs to serve cipi.yml over the API.
const ymlRequires = "cipi.yml over the API requires cipi/api 1.35+ and Cipi 5.6.4+ on the server (cipi self-update, cipi api update)"

// ymlAutoWarning is what turning auto-apply on hands over to the repository.
var ymlAutoWarning = []string{
	"With auto-apply on, anyone who can commit to the repository can change this app's",
	"aliases, PHP settings, limits, workers, deploy recipe, scheduled commands,",
	"databases and backup schedule — for a Node app also its mode, Node version,",
	"build and start commands.",
}

type ymlChange struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
}

// ymlPlanData is GET /api/apps/{name}/yml. File is nil when the live release
// has no cipi.yml; Valid and InSync are nil when there is nothing to judge.
type ymlPlanData struct {
	App      string      `json:"app"`
	File     *string     `json:"file"`
	Auto     bool        `json:"auto"`
	Valid    *bool       `json:"valid"`
	InSync   *bool       `json:"in_sync"`
	Errors   []string    `json:"errors"`
	Warnings []string    `json:"warnings"`
	Changes  []ymlChange `json:"changes"`
	Blockers []string    `json:"blockers"`
	Notes    []string    `json:"notes"`
}

// ymlAutoData is the answer of POST /api/apps/{name}/yml/auto/enable|disable.
type ymlAutoData struct {
	App     string  `json:"app"`
	Auto    bool    `json:"auto"`
	Sudoers bool    `json:"sudoers"`
	File    *string `json:"file"`
}

var ymlCmd = &cobra.Command{
	Use:   "yml <app>",
	Short: "Plan and apply the cipi.yml of an app",
	Long: `An app can ship a cipi.yml in its repository describing the server state it
expects: aliases, redirects, proxies, PHP version and settings, limits, extra
databases, workers, healthcheck, deploy options, crons, backups and post-deploy
steps. These commands mirror "cipi yml" on the host.

  cipi-cli yml <app>                     what applying would change (same as plan)
  cipi-cli yml plan <app>                the same
  cipi-cli yml apply <app> [-y]          apply it (async job)
  cipi-cli yml auto <app> on|off|status  apply after every successful deploy

The file is read from the live release, so commit it and deploy first.
plan exits with status 1 when there is no file, it is not valid or something
blocks the apply; apply exits with status 1 unless the outcome is applied or
unchanged.

Requires Cipi 5.6.4+, cipi/api 1.35+ and the yml-view (plan, auto status) /
yml-manage (apply, auto on|off) token abilities. Existing tokens do not get
them automatically.

` + multiServerTip,
	Example: `  cipi-cli yml myapp
  cipi-cli prod yml plan myapp --json
  cipi-cli yml apply myapp
  cipi-cli yml apply myapp -y
  cipi-cli yml auto myapp on`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runYmlPlan(cmd, args[0])
	},
}

var ymlPlanCmd = &cobra.Command{
	Use:     "plan <app>",
	Aliases: []string{"show", "status", "diff"},
	Short:   "Show what applying cipi.yml would change",
	Long: `Read the cipi.yml of the live release and show what applying it would change
— "cipi yml plan" on the host, read from GET /api/apps/{name}/yml.

Prints the file, whether auto-apply is on, validation errors and warnings, the
changes, what blocks the apply and what happens after the next deploy.

Exits with status 1 when there is no file, it is not valid or something blocks
the apply (with --json the data is printed and the exit status is 0).

Requires the yml-view ability.

  cipi-cli yml plan myapp
  cipi-cli yml plan myapp --json`,
	Example: `  cipi-cli yml plan myapp
  cipi-cli prod yml plan myapp
  cipi-cli yml plan myapp --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runYmlPlan(cmd, args[0])
	},
}

var ymlApplyCmd = &cobra.Command{
	Use:   "apply <app>",
	Short: "Apply cipi.yml to the server (async job)",
	Long: `Apply the cipi.yml of the live release — "cipi yml apply" on the host.

Shows the plan and prompts for confirmation unless -y / --yes is passed (the
preview needs the yml-view ability; without it the CLI only asks). Then waits
for the app-yml-apply job and prints its outcome: applied, unchanged, partial
(some changes failed), blocked, invalid or no-file.

Exits with status 1 unless the outcome is applied or unchanged.

Requires the yml-manage ability.

  cipi-cli yml apply myapp
  cipi-cli yml apply myapp -y
  cipi-cli yml apply myapp -y --json`,
	Example: `  cipi-cli yml apply myapp
  cipi-cli prod yml apply myapp -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		yes, _ := cmd.Flags().GetBool("yes")
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		client, err := mustClient()
		if err != nil {
			return err
		}

		if !yes {
			if !jsonFlag {
				plan, _, err := fetchYmlPlan(client, app)
				switch {
				case isHTTPStatus(err, 403):
					output.Warn("No preview: %s", ymlErrorHint(err, "yml-view"))
				case err != nil:
					output.Error("Failed to read cipi.yml: %s", ymlErrorHint(err, "yml-view"))
					return err
				default:
					printYmlPlan(plan)
					if perr := ymlPlanProblem(plan); perr != nil {
						output.Error("Nothing was applied: %s", perr)
						fmt.Println()
						return perr
					}
					if plan.InSync != nil && *plan.InSync {
						return nil
					}
				}
			}
			if !output.Confirm(fmt.Sprintf("Apply cipi.yml to '%s'?", app)) {
				output.Warn("Aborted")
				return nil
			}
		}

		output.Info("Applying cipi.yml to '%s'...", app)
		job, err := client.DoAsyncAndWaitResult("POST", appAPIPath(app, "/yml/apply"), nil)
		result, ok := jobResultMap(job)
		if !ok {
			if err != nil {
				output.Error("cipi.yml apply failed: %s", ymlErrorHint(err, "yml-manage"))
				return err
			}
			if jsonFlag {
				output.PrintJSON(map[string]interface{}{"data": nil})
				return nil
			}
			output.Success("cipi.yml applied to '%s'", app)
			fmt.Println()
			return nil
		}

		if jsonFlag {
			output.PrintJSON(map[string]interface{}{"data": result})
		} else {
			printYmlApplyResult(result, app)
		}
		if !ymlApplySucceeded(result) {
			return fmt.Errorf("cipi.yml apply on %s: %s", app, formatFieldValue(result["outcome"]))
		}
		return nil
	},
}

var ymlAutoCmd = &cobra.Command{
	Use:   "auto <app> [on|off|status]",
	Short: "Apply cipi.yml after every successful deploy (on, off, status)",
	Long: `Turn on or off the automatic apply of cipi.yml after every successful deploy
of the app — CLI deploy, panel deploy and Git webhook alike — or show whether
it is on ("cipi yml auto" on the host). Without on|off the status is shown.

Turning it on means anyone who can commit to the repository can change the
app's aliases, PHP settings, limits, workers, deploy recipe, scheduled
commands, databases and backup schedule (for a Node app also its mode, Node
version, build and start commands): the CLI says so and prompts for
confirmation unless -y / --yes is passed.

on and off need the yml-manage ability, status needs yml-view.

  cipi-cli yml auto myapp
  cipi-cli yml auto myapp on
  cipi-cli yml auto myapp off`,
	Example: `  cipi-cli yml auto myapp status
  cipi-cli yml auto myapp on -y
  cipi-cli prod yml auto myapp off
  cipi-cli yml auto myapp --json`,
	Args: cobra.RangeArgs(1, 2),
	ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		if len(args) == 1 {
			return []string{"on", "off", "status"}, cobra.ShellCompDirectiveNoFileComp
		}
		return nil, cobra.ShellCompDirectiveNoFileComp
	},
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		mode := "status"
		if len(args) == 2 {
			mode = strings.ToLower(args[1])
		}
		switch mode {
		case "on", "enable":
			mode = "on"
		case "off", "disable":
			mode = "off"
		case "status":
		default:
			return fmt.Errorf("unknown mode %q — use on, off or status", args[1])
		}
		yes, _ := cmd.Flags().GetBool("yes")
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true

		client, err := mustClient()
		if err != nil {
			return err
		}

		if mode == "status" {
			plan, _, err := fetchYmlPlan(client, app)
			if err != nil {
				output.Error("Failed to read cipi.yml: %s", ymlErrorHint(err, "yml-view"))
				return err
			}
			if jsonFlag {
				output.PrintJSON(map[string]interface{}{"data": map[string]interface{}{
					"app":  plan.App,
					"auto": plan.Auto,
					"file": plan.File,
				}})
				return nil
			}
			output.Header(fmt.Sprintf("cipi.yml for '%s'", app))
			printYmlFileAndAuto(plan.File, plan.Auto, app)
			fmt.Println()
			return nil
		}

		if mode == "on" {
			if !jsonFlag {
				fmt.Println()
				for _, line := range ymlAutoWarning {
					output.Warn("%s", line)
				}
			}
			if !yes && !output.Confirm(fmt.Sprintf("Turn on automatic cipi.yml apply for '%s'?", app)) {
				output.Warn("Aborted")
				return nil
			}
		}

		action := "enable"
		if mode == "off" {
			action = "disable"
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Post(appAPIPath(app, "/yml/auto/"+action), nil, &result); err != nil {
			output.Error("Failed to %s automatic cipi.yml apply: %s", action, ymlErrorHint(err, "yml-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data ymlAutoData
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse the answer: %s", err)
			return err
		}

		if mode == "on" {
			output.Success("cipi.yml will be applied after every successful deploy of '%s'", app)
			output.Info("Every path: cipi-cli deploy, the panel and the Git webhook.")
			if !data.Sudoers {
				output.Warn("The sudoers rule is missing — webhook deploys skip cipi.yml; run again: cipi-cli yml auto %s on", app)
			}
			if data.File == nil {
				output.Dim.Println("  No cipi.yml in the live release yet — the first deploy that ships one applies it.")
			}
		} else {
			output.Success("Automatic cipi.yml apply disabled for '%s'", app)
			if data.Sudoers {
				output.Warn("The sudoers rule is still in place — run again: cipi-cli yml auto %s off", app)
			}
		}
		fmt.Println()
		return nil
	},
}

func runYmlPlan(cmd *cobra.Command, app string) error {
	cmd.SilenceUsage = true
	cmd.SilenceErrors = true

	client, err := mustClient()
	if err != nil {
		return err
	}

	plan, raw, err := fetchYmlPlan(client, app)
	if err != nil {
		output.Error("Failed to read cipi.yml: %s", ymlErrorHint(err, "yml-view"))
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]json.RawMessage{"data": raw})
		return nil
	}

	printYmlPlan(plan)
	if perr := ymlPlanProblem(plan); perr != nil {
		return perr
	}
	if plan.InSync == nil || !*plan.InSync {
		output.Dim.Printf("  Apply it: cipi-cli yml apply %s\n", app)
		fmt.Println()
	}
	return nil
}

func fetchYmlPlan(client *api.Client, app string) (*ymlPlanData, json.RawMessage, error) {
	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := client.Get(appAPIPath(app, "/yml"), &result); err != nil {
		return nil, nil, err
	}
	var plan ymlPlanData
	if err := json.Unmarshal(result.Data, &plan); err != nil {
		return nil, nil, fmt.Errorf("parsing cipi.yml status: %w", err)
	}
	if plan.App == "" {
		plan.App = app
	}
	return &plan, result.Data, nil
}

// ymlPlanProblem says why a plan cannot be applied as it is: no file, a file
// that does not validate, or blockers. nil when apply may proceed.
func ymlPlanProblem(plan *ymlPlanData) error {
	switch {
	case plan.File == nil:
		return fmt.Errorf("no cipi.yml in the live release of %s", plan.App)
	case plan.Valid != nil && !*plan.Valid:
		return fmt.Errorf("the cipi.yml of %s is not valid", plan.App)
	case len(plan.Blockers) > 0:
		return fmt.Errorf("%d item(s) block the apply on %s", len(plan.Blockers), plan.App)
	}
	return nil
}

func printYmlPlan(plan *ymlPlanData) {
	output.Header(fmt.Sprintf("cipi.yml for '%s'", plan.App))
	printYmlFileAndAuto(plan.File, plan.Auto, plan.App)

	if plan.File == nil {
		fmt.Println()
		output.Warn("No cipi.yml in the live release — commit one at the root of the repository and deploy")
		output.Dim.Printf("  Start from the app as it is, on the host: cipi yml generate %s --save\n", plan.App)
		fmt.Println()
		return
	}

	switch {
	case plan.Valid == nil:
		output.KeyValue(nil, "Valid", "—")
	case *plan.Valid:
		output.KeyValue(nil, "Valid", output.Green.Sprint("yes"))
	default:
		output.KeyValue(nil, "Valid", output.Red.Sprint("no"))
	}
	if plan.InSync != nil {
		if *plan.InSync {
			output.KeyValue(nil, "In sync", output.Green.Sprint("yes"))
		} else {
			output.KeyValue(nil, "In sync", output.Yellow.Sprint("no"))
		}
	}
	fmt.Println()

	ymlList("Errors", plan.Errors, output.Red.Sprint("✗"))
	ymlList("Warnings", plan.Warnings, output.Yellow.Sprint("!"))
	ymlList("Blocked", plan.Blockers, output.Red.Sprint("✗"))
	ymlList("After deploy", plan.Notes, output.Dim.Sprint("•"))

	summaries := make([]string, 0, len(plan.Changes))
	for _, c := range plan.Changes {
		s := c.Summary
		if s == "" {
			s = c.Kind
		}
		summaries = append(summaries, s)
	}
	ymlList("Would change", summaries, output.Cyan.Sprint("→"))

	if plan.InSync != nil && *plan.InSync {
		output.Success("Nothing to do — the server already matches cipi.yml")
		fmt.Println()
	}
}

func printYmlFileAndAuto(file *string, auto bool, app string) {
	if file != nil {
		output.KeyValue(nil, "File", *file)
	} else {
		output.KeyValue(nil, "File", output.Dim.Sprint("not present in the live release"))
	}
	if auto {
		output.KeyValue(nil, "Auto-apply", output.Green.Sprint("on — after every successful deploy (CLI, panel and webhook)"))
	} else {
		output.KeyValue(nil, "Auto-apply", output.Dim.Sprint("off — deploys ignore the file"))
		output.KeyValue(nil, "", output.Dim.Sprintf("turn it on: cipi-cli yml auto %s on", app))
	}
}

func ymlList(title string, items []string, bullet string) {
	if len(items) == 0 {
		return
	}
	output.Bold.Printf("  %s\n", title)
	for _, item := range items {
		fmt.Printf("    %s %s\n", bullet, item)
	}
	fmt.Println()
}

func printYmlApplyResult(result map[string]interface{}, app string) {
	r := describeYmlApplyResult(result, app)
	fmt.Println()
	if r.OK {
		output.Success("%s", r.Message)
	} else {
		output.Error("%s", r.Message)
	}
	for _, d := range r.Details {
		if r.OK {
			fmt.Printf("    %s %s\n", output.Dim.Sprint("•"), d)
		} else {
			fmt.Printf("    %s %s\n", output.Red.Sprint("✗"), d)
		}
	}
	if r.Hint != "" {
		output.Dim.Printf("  %s\n", r.Hint)
	}
	fmt.Println()
}

// ymlErrorHint explains why a cipi.yml endpoint refused: a 403 names the
// ability the token lacks, a missing route, a 404 that is not about the app
// or a 503 point at the versions the feature needs.
func ymlErrorHint(err error, ability string) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	abilityHint := fmt.Sprintf("%s — the API token lacks the %s ability (existing tokens do not get it: create one on the server with cipi api token create)", msg, ability)

	var apiErr *api.APIError
	if !errors.As(err, &apiErr) {
		// Answers that are not JSON: "API error (HTTP 404): …".
		switch {
		case strings.Contains(msg, "HTTP 403"):
			return abilityHint
		case strings.Contains(msg, "HTTP 404"), strings.Contains(msg, "HTTP 503"):
			return msg + " — " + ymlRequires
		}
		return msg
	}

	switch {
	case apiErr.Status == 403:
		return abilityHint
	case apiErr.IsRouteNotFound():
		return msg + " — " + ymlRequires
	case apiErr.Status == 404 && !strings.Contains(msg, "App '"):
		return msg + " — " + ymlRequires
	case apiErr.Status == 503:
		return msg + " — " + ymlRequires
	}
	return msg
}

func isHTTPStatus(err error, status int) bool {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) {
		return apiErr.Status == status
	}
	return err != nil && strings.Contains(err.Error(), fmt.Sprintf("HTTP %d", status))
}

func init() {
	ymlApplyCmd.Flags().BoolP("yes", "y", false, "Skip the preview and confirmation")
	ymlAutoCmd.Flags().BoolP("yes", "y", false, "Skip confirmation when turning auto-apply on")

	ymlCmd.AddCommand(ymlPlanCmd, ymlApplyCmd, ymlAutoCmd)
	rootCmd.AddCommand(ymlCmd)
}
