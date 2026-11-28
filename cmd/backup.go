package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// backupsMinAPI is the cipi/api release with the backup endpoints; the server
// also needs Cipi 5.9.0 (it answers 503 and says so on an older core).
const backupsMinAPI = "1.39.0"

// backupListTimeout replaces the default 30 s client timeout where the server
// lists every bucket (runs, destination test).
const backupListTimeout = 120 * time.Second

type backupDatabase struct {
	Engine string `json:"engine"`
	Name   string `json:"name"`
}

type backupDestination struct {
	Name     string   `json:"name"`
	Type     string   `json:"type"`
	Path     string   `json:"path"`
	Bucket   string   `json:"bucket"`
	Region   *string  `json:"region"`
	Endpoint *string  `json:"endpoint"`
	Default  bool     `json:"default"`
	Profiles []string `json:"profiles"`
}

type backupProfile struct {
	Name            string   `json:"name"`
	Scope           string   `json:"scope"`
	Enabled         bool     `json:"enabled"`
	Cron            *string  `json:"cron"`
	IntervalSeconds *int64   `json:"interval_seconds"`
	Destinations    []string `json:"destinations"`
	Retention       struct {
		Keep  int `json:"keep"`
		Days  int `json:"days"`
		Weeks int `json:"weeks"`
	} `json:"retention"`
	Encrypt bool `json:"encrypt"`
	// ExcludePaths are left out of the app archives (Cipi 5.13.0+).
	ExcludePaths []string `json:"exclude_paths"`
	Covers       struct {
		Apps      []string         `json:"apps"`
		Databases []backupDatabase `json:"databases"`
	} `json:"covers"`
	Last *struct {
		Status     *string `json:"status"`
		Detail     *string `json:"detail"`
		FinishedAt *string `json:"finished_at"`
	} `json:"last"`
	Overdue bool `json:"overdue"`
	Running bool `json:"running"`
}

// backupStatusData is GET /api/backups: what "cipi backup status --json"
// prints, plus where the answer comes from (host, config_file — cipi/api
// 1.39.1+). Destinations never carry their keys.
type backupStatusData struct {
	Configured    bool                `json:"configured"`
	Host          *string             `json:"host"`
	ConfigFile    string              `json:"config_file"`
	LocalDir      *string             `json:"local_dir"`
	EncryptionKey bool                `json:"encryption_key"`
	Destinations  []backupDestination `json:"destinations"`
	Profiles      []backupProfile     `json:"profiles"`
	Uncovered     struct {
		Apps      []string         `json:"apps"`
		Databases []backupDatabase `json:"databases"`
	} `json:"uncovered"`
}

// backupRunsData is GET /api/backups/runs: profile → the runs each
// destination holds, newest first; Errors are the buckets that could not be
// listed.
type backupRunsData struct {
	Profiles map[string][]struct {
		Run          string   `json:"run"`
		Destinations []string `json:"destinations"`
	} `json:"profiles"`
	Errors []string `json:"errors"`
}

var backupCmd = &cobra.Command{
	Use:     "backup",
	Aliases: []string{"backups"},
	Short:   "Backup profiles, runs and destinations",
	Long: `See and manage the backups of the selected server — "cipi backup" on the
host. A profile says what a run takes (apps, databases), when, where to
(destinations) and how long it is kept.

  cipi-cli backup                         status (same as backup status)
  cipi-cli backup status                  destinations, profiles, last runs, what is not covered
  cipi-cli backup runs [--profile P]      the runs each destination holds
  cipi-cli backup run [profile]           start a run now, in the background
  cipi-cli backup verify [profile]        check that the newest run opens (async job)
  cipi-cli backup profile add <name> --dest local,s3 [flags]
  cipi-cli backup profile edit <name> [flags]
  cipi-cli backup profile enable|disable <name>
  cipi-cli backup profile remove <name> [-y]
  cipi-cli backup destination test [name] list each S3 destination with its credentials

Destinations are read-only here: adding one takes an S3 secret key, so
"cipi backup configure" and "cipi backup destination add|edit|remove" are run
on the server, as are fetch, key, prune and "profile remove --purge".

Requires Cipi 5.9.0+, cipi/api 1.39+ and the backups-view (status, runs) /
backups-manage (everything else) token abilities — existing tokens do not get
them.

` + multiServerTip,
	Example: `  cipi-cli backup
  cipi-cli prod backup status --json
  cipi-cli backup runs --profile daily
  cipi-cli backup run daily
  cipi-cli backup profile add daily --dest local,s3 --every 1d --keep 7
  cipi-cli backup profile edit daily --exclude-tables "*.jobs,*.sessions"`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackupStatus()
	},
}

var backupStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Destinations, profiles, last runs and what no profile covers",
	Long: `Show the backup status of the server ("cipi backup status" on the host,
read from GET /api/backups): the destinations (never their keys), every
profile with its schedule, retention, what it covers right now and how its
last run ended, whether it is overdue or running, and the apps and databases
no enabled profile takes.

It changes nothing on the server, so it can be polled. A server where backups
were never configured says so, with its hostname and the file it looked for.

Requires the backups-view ability.

  cipi-cli backup status
  cipi-cli prod backup status --json`,
	Example: `  cipi-cli backup status
  cipi-cli prod backup status
  cipi-cli backup status --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackupStatus()
	},
}

var backupRunsCmd = &cobra.Command{
	Use:     "runs",
	Aliases: []string{"list"},
	Short:   "The runs each destination holds, per profile",
	Long: `List the backup runs each destination holds, newest first, per profile
("cipi backup list" on the host, read from GET /api/backups/runs).

The server lists every bucket, so it can take a few seconds. Buckets that
could not be listed are reported after the runs.

Requires the backups-view ability.

  cipi-cli backup runs
  cipi-cli backup runs --profile daily`,
	Example: `  cipi-cli backup runs
  cipi-cli prod backup runs --profile daily
  cipi-cli backup runs --json`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		profile, _ := cmd.Flags().GetString("profile")

		client, err := backupListClient()
		if err != nil {
			return err
		}

		path := "/api/backups/runs"
		if profile != "" {
			path += "?profile=" + url.QueryEscape(profile)
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get(path, &result); err != nil {
			output.Error("Failed to list backup runs: %s", featureErrorHint(err, backupsMinAPI, "backups-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		var data backupRunsData
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse backup runs: %s", err)
			return err
		}
		printBackupRuns(&data)
		return nil
	},
}

var backupRunCmd = &cobra.Command{
	Use:   "run [profile]",
	Short: "Start a backup run now, in the background",
	Long: `Start a backup run on the server and return at once ("cipi backup run
--detach" on the host): one profile, or every enabled one when none is given.

The run goes on by itself: it shows as running in "backup status", then as
the profile's last run.

Requires the backups-manage ability.

  cipi-cli backup run
  cipi-cli backup run daily`,
	Example: `  cipi-cli backup run
  cipi-cli prod backup run daily`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Post("/api/backups/run", backupNameBody("profile", args), &result); err != nil {
			output.Error("Failed to start the backup: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		if len(args) == 1 {
			output.Success("Backup of profile '%s' started", args[0])
		} else {
			output.Success("Backup of every enabled profile started")
		}
		output.Dim.Println("  It runs in the background — follow it with: cipi-cli backup status")
		fmt.Println()
		return nil
	},
}

var backupVerifyCmd = &cobra.Command{
	Use:   "verify [profile]",
	Short: "Check that the newest run of a profile opens (async job)",
	Long: `Read the newest run of a profile — or of each one when none is given —
back and check that every archive in it opens ("cipi backup verify" on the
host). The server downloads the run from the bucket when it has no local
copy, so this can take a while; the CLI waits for the job and prints what the
server reported.

Requires the backups-manage ability.

  cipi-cli backup verify
  cipi-cli backup verify daily`,
	Example: `  cipi-cli backup verify
  cipi-cli prod backup verify daily`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if len(args) == 1 {
			output.Info("Verifying the newest run of profile '%s'...", args[0])
		} else {
			output.Info("Verifying the newest run of every profile...")
		}
		job, err := client.DoAsyncAndWaitResult("POST", "/api/backups/verify", backupNameBody("profile", args))
		// The job has no parsed result: the report is the CLI output, also
		// when the verification failed.
		if job != nil {
			printCLIOutput(job.Output)
		}
		if err != nil {
			output.Error("Backup verify failed: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		output.Success("Backup verified")
		fmt.Println()
		return nil
	},
}

var backupProfileCmd = &cobra.Command{
	Use:     "profile",
	Aliases: []string{"profiles"},
	Short:   "Add, edit, enable, disable or remove a backup profile",
	Long: `Manage the backup profiles of the server ("cipi backup profile" on the
host). The profiles and what each covers are listed by "cipi-cli backup
status".

  cipi-cli backup profile add <name> --dest local,s3 [flags]
  cipi-cli backup profile edit <name> [flags]
  cipi-cli backup profile enable <name>
  cipi-cli backup profile disable <name>
  cipi-cli backup profile remove <name> [-y]

Requires the backups-manage ability.`,
}

const backupProfileFlagsHelp = `What a run takes:
  --scope all|files|db          apps and databases, only files, only databases
  --apps a,b                    app names or globs
  --databases shop,tenant_*     database names or globs
  --exclude-databases a,b       databases to leave out
  --exclude-tables "*.jobs"     db.table globs to leave out
  --exclude-paths storage/app/uploads,*.mp4
                                paths left out of the app archives, relative
                                to the app; shop:storage/app/uploads for one
                                app only (Cipi 5.13.0+, cipi/api 1.43+)

When:
  --every 30m|6h|1d             an interval — or
  --cron "0 2 * * *"            a five-field cron expression (not both)

Where to and for how long:
  --dest local,s3,offsite       local, s3 (the default bucket) or named destinations
  --keep N                      runs to keep
  --keep-days N                 days to keep one run a day
  --keep-weeks N                weeks to keep one run a week
  --encrypt                     client-side AES-256 (the key is created on the
                                server on first use; --encrypt=false turns it off)

The three --keep flags are one retention policy: giving any of them sets the
others to 0, so pass together the ones you want.`

var backupProfileAddCmd = &cobra.Command{
	Use:     "add <name>",
	Aliases: []string{"create"},
	Short:   "Create a backup profile",
	Long: `Create a backup profile ("cipi backup profile add" on the host). The name
is 2 to 32 lowercase letters, digits and dashes, starting with a letter;
--dest is required. What is not given takes the server's defaults.

` + backupProfileFlagsHelp + `

  cipi-cli backup profile add daily --dest local,s3 --every 1d --keep 7
  cipi-cli backup profile add shop-db --scope db --databases "shop,shop_*" \
    --dest offsite --cron "0 */6 * * *" --keep-days 14 --encrypt`,
	Example: `  cipi-cli backup profile add daily --dest local,s3 --every 1d --keep 7
  cipi-cli backup profile add shop-db --scope db --databases "shop,shop_*" \
    --dest offsite --cron "0 */6 * * *" --keep-days 14 --encrypt`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := backupProfileBody(cmd)
		if _, ok := body["destinations"]; !ok {
			output.Error("--dest is required: local, s3 or a named destination")
			return fmt.Errorf("no destination specified")
		}
		body["name"] = args[0]

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Post("/api/backups/profiles", body, &result); err != nil {
			output.Error("Failed to create the backup profile: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}
		output.Success("Backup profile '%s' created", args[0])
		output.Dim.Printf("  What it covers: cipi-cli backup status · Run it now: cipi-cli backup run %s\n", args[0])
		fmt.Println()
		return nil
	},
}

var backupProfileEditCmd = &cobra.Command{
	Use:     "edit <name>",
	Aliases: []string{"update", "set"},
	Short:   "Change a backup profile",
	Long: `Change a backup profile ("cipi backup profile edit" on the host): only
the flags passed change. An empty exclusion list clears it
(--exclude-databases "", --exclude-tables "" or --exclude-paths "").

` + backupProfileFlagsHelp + `

  cipi-cli backup profile edit daily --every 12h
  cipi-cli backup profile edit daily --keep 14 --keep-weeks 8
  cipi-cli backup profile edit daily --exclude-tables ""
  cipi-cli backup profile edit daily --exclude-paths storage/app/uploads`,
	Example: `  cipi-cli backup profile edit daily --every 12h
  cipi-cli backup profile edit daily --keep 14 --keep-weeks 8
  cipi-cli prod backup profile edit daily --exclude-tables "*.jobs,*.sessions"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		body := backupProfileBody(cmd)
		if len(body) == 0 {
			output.Error("Pass at least one flag to change")
			return fmt.Errorf("no backup profile fields specified")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Put(backupProfilePath(args[0], ""), body, &result); err != nil {
			output.Error("Failed to change the backup profile: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}
		output.Success("Backup profile '%s' updated", args[0])
		fmt.Println()
		return nil
	},
}

var backupProfileRemoveCmd = &cobra.Command{
	Use:     "remove <name>",
	Aliases: []string{"rm", "delete"},
	Short:   "Remove a backup profile (its archives stay)",
	Long: `Remove a backup profile ("cipi backup profile remove" on the host). The
archives it made stay on every destination: the server says how many and for
how long. Deleting them too is "cipi backup profile remove <name> --purge" on
the server.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli backup profile remove daily
  cipi-cli backup profile remove daily -y`,
	Example: `  cipi-cli backup profile remove daily
  cipi-cli prod backup profile remove daily -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Remove backup profile '%s'? Its archives stay on the destinations.", args[0])) {
			output.Warn("Aborted")
			return nil
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Delete(backupProfilePath(args[0], ""), nil, &result); err != nil {
			output.Error("Failed to remove the backup profile: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}
		var data struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(result.Data, &data)
		output.Success("Backup profile '%s' removed", args[0])
		printCLIOutput(data.Message)
		fmt.Println()
		return nil
	},
}

var backupProfileEnableCmd = &cobra.Command{
	Use:     "enable <name>",
	Short:   "Enable a backup profile",
	Example: `  cipi-cli backup profile enable daily`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackupProfileToggle(args[0], true)
	},
}

var backupProfileDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable a backup profile (it stops running, nothing is deleted)",
	Long: `Disable a backup profile: it stops running on its schedule, and what it
covered counts as not covered in "backup status". Nothing is deleted.

  cipi-cli backup profile disable daily`,
	Example: `  cipi-cli backup profile disable daily`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return runBackupProfileToggle(args[0], false)
	},
}

var backupDestinationCmd = &cobra.Command{
	Use:     "destination",
	Aliases: []string{"destinations", "dest"},
	Short:   "Where archives go: S3 buckets and SFTP servers",
	Long: `Backup destinations: "local" (this server), "s3" (the default bucket — set
with "cipi-cli backup configure"), and more with a name of their own — S3
buckets and SFTP servers. They are listed by "cipi-cli backup status".

  cipi-cli backup destination add offsite --bucket acme-offsite --key AKIA… --endpoint https://…
  cipi-cli backup destination add box --host u1234.your-storagebox.de --port 23 --user u1234
  cipi-cli backup destination edit offsite --region eu-west-1
  cipi-cli backup destination test [name]
  cipi-cli backup destination key box
  cipi-cli backup destination remove offsite

A secret key is asked at a prompt, or read from a file or stdin — never a
flag — and sent once: the server reads it on its standard input.`,
}

var backupDestinationTestCmd = &cobra.Command{
	Use:   "test [name]",
	Short: "Check that the server reaches each destination",
	Long: `Check that the server reaches its destinations — each one, or the one
named ("cipi backup destination test" on the host): an S3 bucket is listed
with its own credentials, an SFTP server gets a file written and removed.
"s3" is the default bucket.

Exits with status 1 when a destination cannot be listed.

Requires the backups-manage ability.

  cipi-cli backup destination test
  cipi-cli backup destination test offsite`,
	Example: `  cipi-cli backup destination test
  cipi-cli prod backup destination test offsite`,
	Args:          cobra.MaximumNArgs(1),
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := backupListClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Post("/api/backups/destinations/test", backupNameBody("name", args), &result); err != nil {
			output.Error("Failed to test the destinations: %s", featureErrorHint(err, backupsMinAPI, "backups-manage"))
			return err
		}

		var data struct {
			OK     bool   `json:"ok"`
			Output string `json:"output"`
		}
		if err := json.Unmarshal(result.Data, &data); err != nil {
			output.Error("Failed to parse the answer: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
		} else {
			fmt.Println()
			printCLIOutput(data.Output)
			if data.OK {
				output.Success("Destinations reachable")
			} else {
				output.Error("A destination could not be listed")
			}
			fmt.Println()
		}
		if !data.OK {
			return fmt.Errorf("backup destination test failed")
		}
		return nil
	},
}

func runBackupStatus() error {
	client, err := mustClient()
	if err != nil {
		return err
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := client.Get("/api/backups", &result); err != nil {
		output.Error("Failed to read backup status: %s", featureErrorHint(err, backupsMinAPI, "backups-view"))
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
		return nil
	}

	var data backupStatusData
	if err := json.Unmarshal(result.Data, &data); err != nil {
		output.Error("Failed to parse backup status: %s", err)
		return err
	}
	printBackupStatus(&data)
	return nil
}

func runBackupProfileToggle(name string, enable bool) error {
	action, done := "enable", "enabled"
	if !enable {
		action, done = "disable", "disabled"
	}

	client, err := mustClient()
	if err != nil {
		return err
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := client.Post(backupProfilePath(name, "/"+action), nil, &result); err != nil {
		output.Error("Failed to %s the backup profile: %s", action, featureErrorHint(err, backupsMinAPI, "backups-manage"))
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
		return nil
	}
	output.Success("Backup profile '%s' %s", name, done)
	fmt.Println()
	return nil
}

func backupListClient() (*api.Client, error) {
	client, err := mustClient()
	if err != nil {
		return nil, err
	}
	client.HTTPClient.Timeout = backupListTimeout
	return client, nil
}

func backupProfilePath(name, suffix string) string {
	return "/api/backups/profiles/" + url.PathEscape(name) + suffix
}

// backupNameBody is the optional body of run / verify / destination test: the
// one profile or destination named, or no body at all for every one of them.
func backupNameBody(field string, args []string) interface{} {
	if len(args) == 1 {
		return map[string]string{field: args[0]}
	}
	return nil
}

// backupProfileBody turns the flags that were passed into the fields of a
// profile. Only those are sent: on edit the others stay as they are, on add
// they take the server's defaults. A list flag passed empty is sent as an
// empty list, which clears an exclusion on edit.
func backupProfileBody(cmd *cobra.Command) map[string]interface{} {
	flags := cmd.Flags()
	body := map[string]interface{}{}

	for flag, field := range map[string]string{"scope": "scope", "every": "every", "cron": "cron"} {
		if flags.Changed(flag) {
			v, _ := flags.GetString(flag)
			body[field] = v
		}
	}
	for flag, field := range map[string]string{
		"apps":              "apps",
		"databases":         "databases",
		"exclude-databases": "exclude_databases",
		"exclude-tables":    "exclude_tables",
		"exclude-paths":     "exclude_paths",
		"dest":              "destinations",
	} {
		if flags.Changed(flag) {
			v, _ := flags.GetStringSlice(flag)
			if v == nil {
				v = []string{}
			}
			body[field] = v
		}
	}
	for flag, field := range map[string]string{"keep": "keep", "keep-days": "keep_days", "keep-weeks": "keep_weeks"} {
		if flags.Changed(flag) {
			v, _ := flags.GetInt(flag)
			body[field] = v
		}
	}
	if flags.Changed("encrypt") {
		v, _ := flags.GetBool("encrypt")
		body["encrypt"] = v
	}
	return body
}

func addBackupProfileFlags(cmd *cobra.Command) {
	cmd.Flags().String("scope", "", "What a run takes: all, files or db")
	cmd.Flags().StringSlice("apps", nil, "App names or globs (comma-separated)")
	cmd.Flags().StringSlice("databases", nil, "Database names or globs, e.g. shop,tenant_*")
	cmd.Flags().StringSlice("exclude-databases", nil, `Databases to leave out ("" clears the list on edit)`)
	cmd.Flags().StringSlice("exclude-tables", nil, `db.table globs to leave out, e.g. "*.jobs" ("" clears the list on edit)`)
	cmd.Flags().StringSlice("exclude-paths", nil, `Paths left out of the app archives, e.g. storage/app/uploads,*.mp4 or shop:public/media ("" clears the list on edit; Cipi 5.13.0+)`)
	cmd.Flags().String("every", "", "Interval: 30m, 6h, 1d (or --cron)")
	cmd.Flags().String("cron", "", `Five-field cron expression, e.g. "0 2 * * *" (or --every)`)
	cmd.Flags().StringSlice("dest", nil, "Destinations: local, s3 or named ones (comma-separated)")
	cmd.Flags().Int("keep", 0, "Runs to keep")
	cmd.Flags().Int("keep-days", 0, "Days to keep one run a day")
	cmd.Flags().Int("keep-weeks", 0, "Weeks to keep one run a week")
	cmd.Flags().Bool("encrypt", false, "Encrypt archives client-side (AES-256); --encrypt=false turns it off")
}

func printBackupStatus(data *backupStatusData) {
	title := "Backups"
	host := ""
	if data.Host != nil && *data.Host != "" {
		host = *data.Host
		title += " — " + host
	}
	output.Header(title)

	if !data.Configured {
		where := "this server"
		if host != "" {
			where = host
		}
		output.Warn("Backups are not configured on %s", where)
		if data.ConfigFile != "" {
			output.Dim.Printf("  The server looked for %s\n", data.ConfigFile)
		}
		output.Dim.Println("  Configure them on the server: cipi backup configure")
		fmt.Println()
		return
	}

	if data.ConfigFile != "" {
		output.KeyValue(nil, "Config file", data.ConfigFile)
	}
	if data.LocalDir != nil && *data.LocalDir != "" {
		output.KeyValue(nil, "Local dir", *data.LocalDir)
	}
	if data.EncryptionKey {
		output.KeyValue(nil, "Encryption key", "present")
	} else {
		output.KeyValue(nil, "Encryption key", output.Dim.Sprint("not created yet"))
	}
	fmt.Println()

	output.Bold.Println("  Destinations")
	if len(data.Destinations) == 0 {
		output.Dim.Println("  None — add one on the server: cipi backup destination add")
		fmt.Println()
	} else {
		t := output.NewTable("NAME", "TYPE", "TARGET", "REGION", "DEFAULT", "PROFILES")
		for _, d := range data.Destinations {
			def := ""
			if d.Default {
				def = output.Green.Sprint("✓")
			}
			t.Row(d.Name, d.Type, backupDestinationTarget(d), strOrDash(d.Region), def, joinOrDash(d.Profiles))
		}
		t.Flush()
	}

	output.Bold.Println("  Profiles")
	if len(data.Profiles) == 0 {
		output.Dim.Println("  None — create one: cipi-cli backup profile add <name> --dest local")
		fmt.Println()
	} else {
		t := output.NewTable("NAME", "SCOPE", "SCHEDULE", "DEST", "KEEP", "ENCRYPT", "COVERS", "LAST RUN", "STATE")
		for _, p := range data.Profiles {
			t.Row(
				p.Name,
				dash(p.Scope),
				backupSchedule(p.Cron, p.IntervalSeconds),
				joinOrDash(p.Destinations),
				backupRetention(p.Retention.Keep, p.Retention.Days, p.Retention.Weeks),
				formatFieldValue(p.Encrypt),
				fmt.Sprintf("%d app(s), %d db(s)", len(p.Covers.Apps), len(p.Covers.Databases)),
				backupLastRun(&p),
				backupProfileState(&p),
			)
		}
		t.Flush()
		for _, p := range data.Profiles {
			if len(p.ExcludePaths) > 0 && p.Scope != "db" {
				output.KeyValue(nil, p.Name+" leaves out", strings.Join(p.ExcludePaths, ", "))
			}
		}
		for _, p := range data.Profiles {
			if p.Last != nil && p.Last.Detail != nil && *p.Last.Detail != "" && !backupStatusOK(p.Last.Status) {
				output.Warn("%s: %s", p.Name, *p.Last.Detail)
			}
		}
	}

	if n := len(data.Uncovered.Apps) + len(data.Uncovered.Databases); n > 0 {
		output.Warn("%d item(s) are in no enabled profile", n)
		if len(data.Uncovered.Apps) > 0 {
			output.KeyValue(nil, "Apps", strings.Join(data.Uncovered.Apps, ", "))
		}
		if len(data.Uncovered.Databases) > 0 {
			names := make([]string, 0, len(data.Uncovered.Databases))
			for _, db := range data.Uncovered.Databases {
				names = append(names, backupDatabaseLabel(db))
			}
			output.KeyValue(nil, "Databases", strings.Join(names, ", "))
		}
		fmt.Println()
	}
}

func printBackupRuns(data *backupRunsData) {
	output.Header("Backup runs")
	for _, e := range data.Errors {
		output.Warn("%s", e)
	}
	if len(data.Profiles) == 0 {
		output.Warn("No runs found")
		fmt.Println()
		return
	}

	names := make([]string, 0, len(data.Profiles))
	for name := range data.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		runs := data.Profiles[name]
		output.Bold.Printf("  %s\n", name)
		if len(runs) == 0 {
			output.Dim.Println("  No runs yet")
			fmt.Println()
			continue
		}
		t := output.NewTable("RUN", "DESTINATIONS")
		for _, r := range runs {
			t.Row(r.Run, joinOrDash(r.Destinations))
		}
		t.Flush()
	}
}

func backupDestinationTarget(d backupDestination) string {
	switch {
	case d.Bucket != "" && d.Endpoint != nil && *d.Endpoint != "":
		return fmt.Sprintf("%s (%s)", d.Bucket, *d.Endpoint)
	case d.Bucket != "":
		return d.Bucket
	default:
		return dash(d.Path)
	}
}

// backupSchedule is when a profile runs: its cron expression, else its
// interval the way --every takes it (30m, 6h, 1d).
func backupSchedule(cron *string, intervalSeconds *int64) string {
	if cron != nil && *cron != "" {
		return *cron
	}
	if intervalSeconds == nil || *intervalSeconds <= 0 {
		return "—"
	}
	s := *intervalSeconds
	switch {
	case s%86400 == 0:
		return fmt.Sprintf("every %dd", s/86400)
	case s%3600 == 0:
		return fmt.Sprintf("every %dh", s/3600)
	case s%60 == 0:
		return fmt.Sprintf("every %dm", s/60)
	default:
		return fmt.Sprintf("every %ds", s)
	}
}

// backupRetention names the parts of a retention policy that are set.
func backupRetention(keep, days, weeks int) string {
	var parts []string
	if keep > 0 {
		parts = append(parts, fmt.Sprintf("%d runs", keep))
	}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%d days", days))
	}
	if weeks > 0 {
		parts = append(parts, fmt.Sprintf("%d weeks", weeks))
	}
	return joinOrDash(parts)
}

func backupStatusOK(status *string) bool {
	if status == nil {
		return false
	}
	switch strings.ToLower(*status) {
	case "ok", "success", "completed":
		return true
	}
	return false
}

func backupLastRun(p *backupProfile) string {
	if p.Last == nil || p.Last.Status == nil || *p.Last.Status == "" {
		return output.Dim.Sprint("never")
	}
	label := *p.Last.Status
	if p.Last.FinishedAt != nil && *p.Last.FinishedAt != "" {
		label += " " + *p.Last.FinishedAt
	}
	if backupStatusOK(p.Last.Status) {
		return output.Green.Sprint(label)
	}
	return output.Red.Sprint(label)
}

func backupProfileState(p *backupProfile) string {
	switch {
	case p.Running:
		return output.Cyan.Sprint("running")
	case !p.Enabled:
		return output.Dim.Sprint("disabled")
	case p.Overdue:
		return output.Red.Sprint("overdue")
	default:
		return output.Green.Sprint("enabled")
	}
}

func backupDatabaseLabel(db backupDatabase) string {
	if db.Engine == "" {
		return db.Name
	}
	return db.Engine + "/" + db.Name
}

func strOrDash(s *string) string {
	if s == nil {
		return "—"
	}
	return dash(*s)
}

func joinOrDash(items []string) string {
	if len(items) == 0 {
		return "—"
	}
	return strings.Join(items, ", ")
}

func init() {
	backupRunsCmd.Flags().String("profile", "", "Only this profile")
	backupProfileRemoveCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	addBackupProfileFlags(backupProfileAddCmd)
	addBackupProfileFlags(backupProfileEditCmd)

	backupProfileCmd.AddCommand(backupProfileAddCmd, backupProfileEditCmd, backupProfileEnableCmd, backupProfileDisableCmd, backupProfileRemoveCmd)
	backupDestinationCmd.AddCommand(backupDestinationTestCmd)
	backupCmd.AddCommand(backupStatusCmd, backupRunsCmd, backupRunCmd, backupVerifyCmd, backupProfileCmd, backupDestinationCmd)
	rootCmd.AddCommand(backupCmd)
}
