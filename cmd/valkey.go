package cmd

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// Valkey per app through the API came with Cipi 5.17.0 and cipi/api 1.47
// (abilities services-view, services-manage).
const valkeyMinAPI = "1.47.0"

type valkeyApp struct {
	App      string `json:"app"`
	Uses     string `json:"uses"`
	Port     *int   `json:"port"`
	Instance string `json:"instance"`
	Memory   string `json:"memory"`
	Keys     *int   `json:"keys"`
}

// valkeyList is the data of GET /api/valkey. No password is in it.
type valkeyList struct {
	SharedPort int         `json:"shared_port"`
	Apps       []valkeyApp `json:"apps"`
	Shared     []string    `json:"shared"`
	Isolated   int         `json:"isolated"`
	Flushed    bool        `json:"flushed,omitempty"`
}

type valkeyResponse struct {
	Data valkeyList `json:"data"`
}

// valkeyMoveBody is what isolate and share send: one app or all, and
// copy: false only when the keys are to be left behind.
func valkeyMoveBody(app string, all, noCopy bool) (map[string]interface{}, error) {
	if (app == "") == !all {
		return nil, errors.New("give an app, or --all for every app on the shared instance")
	}
	body := map[string]interface{}{}
	if all {
		body["all"] = true
	} else {
		body["app"] = app
	}
	if noCopy {
		body["copy"] = false
	}
	return body, nil
}

func valkeyFail(action string, err error, ability string) error {
	output.Error("%s: %s", action, featureErrorHint(err, valkeyMinAPI, ability))
	return err
}

func printValkeyMove(result map[string]interface{}) {
	moved, _ := result["moved"].([]interface{})
	for _, item := range moved {
		m, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if str(m, "uses") == "own" {
			line := fmt.Sprintf("%s: its own Valkey on 127.0.0.1:%s", str(m, "app"), str(m, "port"))
			if m["keys_copied"] != nil {
				line += " — " + str(m, "keys_copied") + " key(s) copied"
			}
			output.Success("%s", line)
		} else {
			output.Success("%s: back on the shared Valkey", str(m, "app"))
		}
	}
	if left, _ := result["left"].([]interface{}); len(left) > 0 {
		names := make([]string, 0, len(left))
		for _, l := range left {
			names = append(names, fmt.Sprintf("%v", l))
		}
		output.Warn("Left where they were: %s", strings.Join(names, ", "))
	}
	if can, _ := result["shared_can_be_flushed"].(bool); can {
		output.Info("No app uses the shared instance any more. It still holds the old keys, and every app has seen its password: cipi-cli valkey flush-shared")
	}
}

var valkeyCmd = &cobra.Command{
	Use:   "valkey",
	Short: "Valkey per app: who shares it, give an app its own instance",
	Long: `A Laravel app keeps cache, sessions and queues in Valkey. On the server's
shared instance every app holds the same password: each one can read and
write the others' keys. An app with an instance of its own — its port, its
password, its memory limit — cannot.

"valkey isolate" gives an app (or every app still on the shared one) its
instance: its keys are copied with the app in maintenance mode for the seconds
that takes, its .env is switched and its workers restart. Once no app is left
on the shared instance, "valkey flush-shared" empties it and changes its
password.

No password ever travels through the API.

Requires Cipi 5.17.0+, cipi/api 1.47+ and the services-view / services-manage
abilities.

  cipi-cli valkey list
  cipi-cli valkey isolate shop
  cipi-cli valkey isolate --all
  cipi-cli valkey flush-shared

` + multiServerTip,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return valkeyListCmd.RunE(cmd, args)
	},
}

var valkeyListCmd = &cobra.Command{
	Use:   "list",
	Short: "Every Laravel app with the Valkey it uses",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result valkeyResponse
		if err := client.Get("/api/valkey", &result); err != nil {
			return valkeyFail("Failed to read Valkey", err, "services-view")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Header("Valkey")
		if len(result.Data.Apps) == 0 {
			output.Warn("No Laravel app")
			fmt.Println()
			return nil
		}
		t := output.NewTable("APP", "USES", "INSTANCE", "STATE", "MEMORY", "KEYS")
		for _, a := range result.Data.Apps {
			uses := a.Uses
			switch a.Uses {
			case "own":
				uses = output.Green.Sprint(uses)
			case "shared":
				uses = output.Yellow.Sprint(uses)
			}
			instance, keys := "—", "—"
			if a.Port != nil {
				instance = "127.0.0.1:" + strconv.Itoa(*a.Port)
			}
			if a.Keys != nil {
				keys = strconv.Itoa(*a.Keys)
			}
			t.Row(a.App, uses, instance, dash(a.Instance), dash(a.Memory), keys)
		}
		t.Flush()
		fmt.Println()
		if len(result.Data.Shared) > 0 {
			output.Warn("shared: %s hold the password of the server's Valkey (127.0.0.1:%d) — each can read the others' cache, sessions and queues", strings.Join(result.Data.Shared, ", "), result.Data.SharedPort)
			fmt.Println("    cipi-cli valkey isolate <app>    cipi-cli valkey isolate --all")
			fmt.Println()
		}
		return nil
	},
}

var valkeyIsolateCmd = &cobra.Command{
	Use:   "isolate <app> | --all",
	Short: "Give an app, or every app on the shared instance, its own Valkey (async job)",
	Long: `Give an app its own Valkey instance. Its keys are copied with the app in
maintenance mode for the seconds that takes; its workers are restarted.
--no-copy starts it empty: no cache, no sessions, no queued jobs.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli valkey isolate shop
  cipi-cli valkey isolate --all -y`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		noCopy, _ := cmd.Flags().GetBool("no-copy")
		yes, _ := cmd.Flags().GetBool("yes")
		app := ""
		if len(args) == 1 {
			app = args[0]
		}
		body, err := valkeyMoveBody(app, all, noCopy)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		if !yes {
			prompt := "Give " + app + " its own Valkey instance? It goes into maintenance mode while its keys are copied"
			if all {
				prompt = "Give every app on the shared instance its own Valkey? Each goes into maintenance mode while its keys are copied"
			}
			if !output.Confirm(prompt) {
				output.Info("Aborted")
				return nil
			}
		}
		job, err := client.DoAsyncAndWaitResult("POST", "/api/valkey/isolate", body)
		if err != nil {
			if job != nil && job.Output != "" {
				printCLIOutput(job.Output)
			}
			return valkeyFail("Not moved", err, "services-manage")
		}
		if jsonFlag && job != nil {
			output.PrintJSON(job)
			return nil
		}
		if m, ok := jobResultMap(job); ok {
			printValkeyMove(m)
		} else {
			output.Success("Done")
		}
		fmt.Println()
		return nil
	},
}

var valkeyShareCmd = &cobra.Command{
	Use:   "share <app>",
	Short: "Move an app back to the shared Valkey and remove its instance (async job)",
	Long: `Move an app back to the server's shared Valkey. On it every other app can
read and write this app's cache, sessions and queues.

Prompts for confirmation unless -y / --yes is passed.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		noCopy, _ := cmd.Flags().GetBool("no-copy")
		yes, _ := cmd.Flags().GetBool("yes")
		body, _ := valkeyMoveBody(args[0], false, noCopy)
		client, err := mustClient()
		if err != nil {
			return err
		}
		if !yes && !output.Confirm("Move "+args[0]+" back to the shared Valkey? Every other app on it can then read its cache, sessions and queues") {
			output.Info("Aborted")
			return nil
		}
		job, err := client.DoAsyncAndWaitResult("POST", "/api/valkey/share", body)
		if err != nil {
			if job != nil && job.Output != "" {
				printCLIOutput(job.Output)
			}
			return valkeyFail("Not moved", err, "services-manage")
		}
		if m, ok := jobResultMap(job); ok {
			printValkeyMove(m)
		} else {
			output.Success("Done")
		}
		fmt.Println()
		return nil
	},
}

var valkeyRestartCmd = &cobra.Command{
	Use:   "restart <app>",
	Short: "Restart an app's own Valkey instance",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result valkeyResponse
		if err := client.Post("/api/valkey/"+url.PathEscape(args[0])+"/restart", nil, &result); err != nil {
			return valkeyFail("Restart failed", err, "services-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("The Valkey of %s was restarted", args[0])
		fmt.Println()
		return nil
	},
}

var valkeyFlushSharedCmd = &cobra.Command{
	Use:   "flush-shared",
	Short: "Empty the shared Valkey and change its password",
	Long: `Once no app is left on the shared instance, what it still holds is their old
cache, sessions and jobs — readable with the password every app was given.
This deletes every key in it and changes its password. Refused while an app
still uses it.

The new password stays on the server (cipi credentials): it does not travel
through the API.

Prompts for confirmation unless -y / --yes is passed.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		client, err := mustClient()
		if err != nil {
			return err
		}
		if !yes && !output.Confirm("Delete every key in the shared Valkey and change its password?") {
			output.Info("Aborted")
			return nil
		}
		var result valkeyResponse
		if err := client.Post("/api/valkey/flush-shared", nil, &result); err != nil {
			return valkeyFail("Not emptied", err, "services-manage")
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		output.Success("Shared Valkey emptied; its password was changed (on the server: cipi credentials)")
		fmt.Println()
		return nil
	},
}

func init() {
	valkeyIsolateCmd.Flags().Bool("all", false, "Every app still on the shared instance")
	valkeyIsolateCmd.Flags().Bool("no-copy", false, "Do not copy the app's keys: cache, sessions and queued jobs start empty")
	valkeyIsolateCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	valkeyShareCmd.Flags().Bool("no-copy", false, "Do not copy the app's keys back")
	valkeyShareCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	valkeyFlushSharedCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	valkeyCmd.AddCommand(valkeyListCmd, valkeyIsolateCmd, valkeyShareCmd, valkeyRestartCmd, valkeyFlushSharedCmd)
	rootCmd.AddCommand(valkeyCmd)
}
