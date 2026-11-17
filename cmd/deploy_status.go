package cmd

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// What the deploy status / history endpoints need on the server. An older
// Cipi answers 503 and says so itself; an older cipi/api has no such route.
const (
	deployStatusMinAPI  = "1.37.0"
	deployHistoryMinAPI = "1.39.0"
	deployMaxTail       = 500
	deployMaxLimit      = 500
)

// deployStatusData is GET /api/apps/{name}/deploy/status. Current is the run
// in flight (or the one that never ended), Last how the previous one ended;
// both are nil when there is none. Runs are kept as maps: the server may send
// a release as a string or a number.
type deployStatusData struct {
	App     string                 `json:"app"`
	State   string                 `json:"state"`
	Running bool                   `json:"running"`
	Phase   *string                `json:"phase"`
	Queued  bool                   `json:"queued"`
	Locked  bool                   `json:"locked"`
	Current map[string]interface{} `json:"current"`
	Last    map[string]interface{} `json:"last"`
	Log     string                 `json:"log"`
	Tail    []string               `json:"tail"`
}

type deployHistoryData struct {
	App   string                   `json:"app"`
	Count int                      `json:"count"`
	Runs  []map[string]interface{} `json:"runs"`
	Log   string                   `json:"log"`
}

var deployStatusCmd = &cobra.Command{
	Use:   "status <app>",
	Short: "Is a deploy running right now, and how the last one ended",
	Long: `Say whether a deploy is running for an app right now — whoever started it:
a Git push through the webhook, "cipi deploy" on the server, the panel, this
CLI or a rollback — and how the last one ended ("cipi deploy <app> --status"
on the host, read from GET /api/apps/{name}/deploy/status).

State is one of:
  running      a deploy is in flight (phase: deploy, then post-deploy)
  queued       a webhook push is waiting for the per-minute cron
  interrupted  the last run never ended and nothing is running it — the
               deploy lock may be stuck (cipi-cli deploy unlock <app>)
  idle         nothing is going on

--tail N adds the last N lines of the deploy log (500 at most).

Requires Cipi 5.8.4+, cipi/api 1.37+ and the deploy-manage ability.

  cipi-cli deploy status myapp
  cipi-cli deploy status myapp --tail 40`,
	Example: `  cipi-cli deploy status myapp
  cipi-cli prod deploy status myapp --tail 40
  cipi-cli deploy status myapp --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		tail, _ := cmd.Flags().GetInt("tail")
		if tail < 0 || tail > deployMaxTail {
			output.Error("--tail must be between 0 and %d", deployMaxTail)
			return fmt.Errorf("invalid tail")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		path := appAPIPath(args[0], "/deploy/status")
		if tail > 0 {
			path += fmt.Sprintf("?tail=%d", tail)
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read deploy status: %s", featureErrorHint(err, deployStatusMinAPI, "deploy-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data deployStatusData
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse deploy status: %s", err)
			return err
		}
		if data.App == "" {
			data.App = args[0]
		}
		printDeployStatus(&data)
		return nil
	},
}

var deployHistoryCmd = &cobra.Command{
	Use:   "history <app>",
	Short: "Past deploys and rollbacks with their duration",
	Long: `List the past deploys and rollbacks of an app, newest first: result,
release, duration, trigger and branch ("cipi deploy --history <app>" on the
host, read from GET /api/apps/{name}/deploy/history).

The runs come from the deploy log and its rotated files, so a deploy of last
week — started by a Git push or by anything else — still has its duration.

--limit N sets how many runs to list (default 30, 500 at most).

Requires Cipi 5.9.0+, cipi/api 1.39+ and the apps-view or deploy-manage
ability.

  cipi-cli deploy history myapp
  cipi-cli deploy history myapp --limit 100`,
	Example: `  cipi-cli deploy history myapp
  cipi-cli prod deploy history myapp --limit 100
  cipi-cli deploy history myapp --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		limit, _ := cmd.Flags().GetInt("limit")
		if limit < 1 || limit > deployMaxLimit {
			output.Error("--limit must be between 1 and %d", deployMaxLimit)
			return fmt.Errorf("invalid limit")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		path := appAPIPath(args[0], fmt.Sprintf("/deploy/history?limit=%d", limit))
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to read deploy history: %s", featureErrorHint(err, deployHistoryMinAPI, "apps-view or deploy-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data deployHistoryData
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse deploy history: %s", err)
			return err
		}

		output.Header(fmt.Sprintf("Deploy history: %s", args[0]))
		if len(data.Runs) == 0 {
			output.Warn("No deploys yet")
			fmt.Println()
			return nil
		}
		t := output.NewTable("STARTED", "KIND", "RESULT", "RELEASE", "DURATION", "TRIGGER", "BRANCH")
		for _, run := range data.Runs {
			t.Row(
				str(run, "started_at"),
				str(run, "kind"),
				deployRunResult(run),
				str(run, "release"),
				deployRunDuration(run),
				str(run, "trigger"),
				str(run, "branch"),
			)
		}
		t.Flush()
		output.Dim.Printf("  %d run(s), newest first\n\n", len(data.Runs))
		return nil
	},
}

var deployLastCmd = &cobra.Command{
	Use:   "last",
	Short: "The last deploy of every app",
	Long: `Show the last finished deploy (or rollback) of every app in one call:
result, release, duration, trigger and when it ended ("cipi deploy --last" on
the host, read from GET /api/deploys/last). Apps that were never deployed are
listed as such.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the apps-view or deploy-manage
ability.

  cipi-cli deploy last
  cipi-cli prod deploy last --json`,
	Example: `  cipi-cli deploy last
  cipi-cli prod deploy last
  cipi-cli deploy last --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get("/api/deploys/last", &result); err != nil {
			output.Error("Failed to read the last deploys: %s", featureErrorHint(err, deployHistoryMinAPI, "apps-view or deploy-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		// App name → its last run, nil when the app was never deployed.
		var apps map[string]map[string]interface{}
		if err := json.Unmarshal(result.Data, &apps); err != nil {
			output.Error("Failed to parse the last deploys: %s", err)
			return err
		}

		output.Header("Last deploys")
		if len(apps) == 0 {
			output.Warn("No apps yet")
			fmt.Println()
			return nil
		}
		names := make([]string, 0, len(apps))
		for name := range apps {
			names = append(names, name)
		}
		sort.Strings(names)

		t := output.NewTable("APP", "RESULT", "KIND", "RELEASE", "DURATION", "TRIGGER", "FINISHED")
		for _, name := range names {
			run := apps[name]
			if run == nil {
				t.Row(name, output.Dim.Sprint("never deployed"))
				continue
			}
			t.Row(
				name,
				deployRunResult(run),
				str(run, "kind"),
				str(run, "release"),
				deployRunDuration(run),
				str(run, "trigger"),
				str(run, "finished_at"),
			)
		}
		t.Flush()
		output.Dim.Println("  Every run of one app: cipi-cli deploy history <app>")
		fmt.Println()
		return nil
	},
}

func printDeployStatus(data *deployStatusData) {
	output.Header(fmt.Sprintf("Deploy status: %s", data.App))

	state := data.State
	if data.Phase != nil && *data.Phase != "" {
		state += " — " + *data.Phase
	}
	output.KeyValue(nil, "State", colorDeployState(data.State, state))
	if data.Current != nil {
		output.KeyValue(nil, "Trigger", str(data.Current, "trigger"))
		output.KeyValue(nil, "Branch", str(data.Current, "branch"))
		started := str(data.Current, "started_at")
		if elapsed, ok := intVal(data.Current, "elapsed"); ok {
			started += fmt.Sprintf(" (%s ago)", formatSeconds(elapsed))
		}
		output.KeyValue(nil, "Started", started)
		output.KeyValue(nil, "From release", str(data.Current, "from_release"))
	}
	output.KeyValue(nil, "Queued", formatFieldValue(data.Queued))
	output.KeyValue(nil, "Locked", formatFieldValue(data.Locked))

	switch data.State {
	case "interrupted":
		output.Warn("The last run never ended and nothing is running it — the deploy lock may be stuck: cipi-cli deploy unlock %s", data.App)
	case "queued":
		output.Dim.Println("  A push is waiting for the per-minute cron to pick it up.")
	}
	fmt.Println()

	output.Bold.Println("  Last run")
	if data.Last == nil {
		output.Dim.Println("  No deploy has finished yet")
	} else {
		output.KeyValue(nil, "Result", fmt.Sprintf("%s (%s)", deployRunResult(data.Last), str(data.Last, "kind")))
		output.KeyValue(nil, "Release", str(data.Last, "release"))
		output.KeyValue(nil, "Duration", deployRunDuration(data.Last))
		output.KeyValue(nil, "Trigger", str(data.Last, "trigger"))
		output.KeyValue(nil, "Branch", str(data.Last, "branch"))
		output.KeyValue(nil, "Finished", str(data.Last, "finished_at"))
	}
	if data.Log != "" {
		output.KeyValue(nil, "Log", data.Log)
	}
	fmt.Println()

	if len(data.Tail) > 0 {
		output.Dim.Printf("  Last %d line(s) of the deploy log\n", len(data.Tail))
		for _, line := range data.Tail {
			fmt.Printf("  %s\n", line)
		}
		fmt.Println()
	}
}

func colorDeployState(state, label string) string {
	switch state {
	case "running":
		return output.Cyan.Sprint(label)
	case "queued":
		return output.Yellow.Sprint(label)
	case "interrupted":
		return output.Red.Sprint(label)
	default:
		return label
	}
}

// deployRunResult is the result of a finished run, with the exit code when
// it failed.
func deployRunResult(run map[string]interface{}) string {
	result := str(run, "result")
	switch result {
	case "ok":
		return output.Green.Sprint(result)
	case "failed":
		if exit, ok := intVal(run, "exit"); ok && exit != 0 {
			result = fmt.Sprintf("failed (exit %d)", exit)
		}
		return output.Red.Sprint(result)
	}
	return result
}

func deployRunDuration(run map[string]interface{}) string {
	if d, ok := intVal(run, "duration"); ok {
		return formatSeconds(d)
	}
	return "—"
}

// intVal reads a whole number the API sent as a JSON number.
func intVal(m map[string]interface{}, key string) (int64, bool) {
	v, ok := m[key].(float64)
	if !ok {
		return 0, false
	}
	return int64(v), true
}

func init() {
	deployStatusCmd.Flags().Int("tail", 0, "Also print the last N lines of the deploy log (max 500)")
	deployHistoryCmd.Flags().Int("limit", 30, "How many runs to list (max 500)")

	deployCmd.AddCommand(deployStatusCmd, deployHistoryCmd, deployLastCmd)
}
