package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:     "db",
	Aliases: []string{"database", "dbs"},
	Short:   "Manage databases",
	Long: `Create, backup, restore, and manage MariaDB/PostgreSQL databases on the selected server.

  cipi-cli db engines
  cipi-cli db install pgsql
  cipi-cli db list
  cipi-cli db create mydb [--app myapp]
  cipi-cli db link mydb myapp                      # show it as the app's
  cipi-cli db unlink mydb
  cipi-cli db backup mydb
  cipi-cli db restore mydb --file <path on the server>
  cipi-cli db restore mydb --upload ./mydb.sql.gz  # upload a local dump
  cipi-cli db password mydb
  cipi-cli db delete mydb
  cipi-cli db anonymize myapp --email dev@example.com   # anonymized dump via cipi/agent

` + multiServerTip,
	Example: `  cipi-cli db list
  cipi-cli prod db list
  cipi-cli db create mydb
  cipi-cli db link mydb myapp
  cipi-cli db backup mydb
  cipi-cli db restore mydb --upload ./mydb.sql.gz -y
  cipi-cli db password mydb -y
  cipi-cli db delete mydb -y
  cipi-cli db anonymize myapp --email dev@example.com
  cipi-cli db anonymize download "<signed url>" -o myapp.sql`,
}

var dbEnginesCmd = &cobra.Command{
	Use:     "engines",
	Aliases: []string{"engine"},
	Short:   "List installed database engines",
	Long: `List installed database engines and the server default.

On a server where "cipi db switch mysql" replaced MariaDB with MySQL (Cipi
5.13.0+, run on the server only), the engine is still called mariadb — the
name every command takes — and SERVER says mysql (cipi/api 1.43+).

Requires Cipi 4.8+.

  cipi-cli db engines
  cipi-cli prod db engines`,
	Example: `  cipi-cli db engines
  cipi-cli db engines --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		var result struct {
			Data dbEnginesData `json:"data"`
		}
		if err := client.Get("/api/dbs/engines", &result); err != nil {
			output.Error("Failed to list database engines: %s", apiErrorHint(err, "4.8", "dbs-view"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header("Database engines")
		printDBEngines(result.Data)
		return nil
	},
}

var dbInstallCmd = &cobra.Command{
	Use:   "install <engine>",
	Short: "Install a database engine (mariadb|pgsql)",
	Long: `Install a database engine on the selected server (async job).

Engines: mariadb, pgsql. Requires Cipi 5.0.6+ (API 1.15+) and the dbs-manage
ability. Changing the default engine is host-only ('cipi db default').

  cipi-cli db install pgsql
  cipi-cli prod db install mariadb`,
	Example: `  cipi-cli db install pgsql
  cipi-cli db engines`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		output.Info("Installing database engine '%s'...", args[0])
		body := map[string]string{"engine": args[0]}
		if err := client.DoAsyncAndWait("POST", "/api/dbs/engines/install", body); err != nil {
			output.Error("Engine install failed: %s", apiErrorHint(err, "1.15.0", "dbs-manage"))
			return err
		}

		output.Success("Database engine '%s' installed", args[0])
		fmt.Println()
		return nil
	},
}

var dbListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all databases",
	Long: `List databases on the active/default server with approximate size.

  cipi-cli db list
  cipi-cli prod db list`,
	Example: `  cipi-cli db list
  cipi-cli staging db list
  cipi-cli db list --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		engine, _ := cmd.Flags().GetString("engine")

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		dbs, err := client.ListDatabases(engine)
		if err != nil {
			output.Error("Failed to list databases: %s", err)
			return err
		}

		result := struct {
			Data []map[string]interface{} `json:"data"`
		}{Data: dbs}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		if len(dbs) == 0 {
			output.Warn("No databases found")
			return nil
		}

		output.Header("Databases")
		headers, rows := dbListTable(dbs)
		t := output.NewTable(headers...)
		for _, row := range rows {
			t.Row(row...)
		}
		t.Flush()
		output.Dim.Printf("  Total: %d database(s)\n\n", len(dbs))
		return nil
	},
}

// dbListTable lays out GET /api/dbs. ENGINE and APP are shown only when the
// server sends them: the app a database belongs to comes with Cipi 5.9.0+,
// with how it is tied to it — linked (db link), own (created with the app) or
// namespace (<app>_…).
func dbListTable(dbs []map[string]interface{}) ([]string, [][]string) {
	hasEngine, hasApp := false, false
	for _, db := range dbs {
		if _, ok := db["engine"]; ok {
			hasEngine = true
		}
		if _, ok := db["app"]; ok {
			hasApp = true
		}
	}

	headers := []string{"NAME"}
	if hasEngine {
		headers = append(headers, "ENGINE")
	}
	headers = append(headers, "SIZE")
	if hasApp {
		headers = append(headers, "APP")
	}

	rows := make([][]string, 0, len(dbs))
	for _, db := range dbs {
		row := []string{str(db, "name")}
		if hasEngine {
			row = append(row, str(db, "engine"))
		}
		row = append(row, dbSize(db))
		if hasApp {
			app := str(db, "app")
			if link, _ := db["link"].(string); link != "" && app != "—" {
				app += " (" + link + ")"
			}
			row = append(row, app)
		}
		rows = append(rows, row)
	}
	return headers, rows
}

// dbSize is the size of a database: the text an older core prints, else the
// size_mb figure of the JSON list (Cipi 5.9.0+).
func dbSize(db map[string]interface{}) string {
	if s, _ := db["size"].(string); s != "" {
		return s
	}
	if mb, ok := db["size_mb"].(float64); ok {
		return formatMB(&mb)
	}
	return "—"
}

var dbCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database",
	Long: `Create a new database on the selected server.

Use --engine to pick mariadb or pgsql (Cipi 4.8+).

Use --app to show the database as an app's: db list and apps show then list
it under that app. It is a label — the database keeps its own user and
password and stays when the app is deleted (Cipi 5.9.0+ / API 1.39+; see
also db link).

  cipi-cli db create mydb
  cipi-cli prod db create mydb
  cipi-cli db create shop_reports --app shop`,
	Example: `  cipi-cli db create mydb
  cipi-cli staging db create client_a
  cipi-cli db create shop_reports --app shop`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		engine, _ := cmd.Flags().GetString("engine")
		app, _ := cmd.Flags().GetString("app")
		body := map[string]string{
			"name": args[0],
		}
		if engine != "" {
			body["engine"] = engine
		}
		if app != "" {
			body["app"] = app
		}

		output.Info("Creating database '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", "/api/dbs", body); err != nil {
			output.Error("Failed to create database: %s", err)
			return err
		}

		output.Success("Database '%s' created successfully", args[0])
		if app != "" {
			warnIfDatabaseNotLinked(client, args[0], engine, app)
		}
		fmt.Println()
		return nil
	},
}

// warnIfDatabaseNotLinked says when the server did not tie a new database to
// the app: a cipi/api older than 1.39 accepts "app" and drops it. The list
// is read back; when it cannot be read nothing is said.
func warnIfDatabaseNotLinked(client *api.Client, name, engine, app string) {
	dbs, err := client.ListDatabases(engine)
	if err != nil {
		return
	}
	for _, db := range dbs {
		if str(db, "name") != name {
			continue
		}
		if got, _ := db["app"].(string); got != app {
			output.Warn("The server did not link '%s' to app '%s' (needs Cipi 5.9.0+ / API 1.39+) — the database was created unlinked", name, app)
		}
		return
	}
}

var dbLinkCmd = &cobra.Command{
	Use:   "link <database> <app>",
	Short: "Show a database as an app's",
	Long: `Tie a database to an app ("cipi db link" on the host): db list and apps
show then list it under that app.

It is a label in the app's entry: the database keeps its own user and
password, and stays when the app is deleted. A database is linked to one app
at most. Use --engine when the name exists on both engines.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the dbs-manage ability.

  cipi-cli db link shop_reports shop
  cipi-cli db link shop_reports shop --engine pgsql`,
	Example: `  cipi-cli db link shop_reports shop
  cipi-cli prod db link shop_reports shop --engine pgsql`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		engine, _ := cmd.Flags().GetString("engine")

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]string{"app": args[1]}
		if engine != "" {
			body["engine"] = engine
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Post(dbAPIPath(args[0], "/link"), body, &result); err != nil {
			output.Error("Failed to link the database: %s", featureErrorHint(err, dbLinkMinAPI, "dbs-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}
		output.Success("Database '%s' linked to app '%s'", args[0], args[1])
		fmt.Println()
		return nil
	},
}

var dbUnlinkCmd = &cobra.Command{
	Use:   "unlink <database>",
	Short: "Remove the link between a database and its app",
	Long: `Remove the link between a database and its app ("cipi db unlink" on the
host). Only the label goes: the database, its user and its data stay. Use
--engine when the name exists on both engines.

Requires Cipi 5.9.0+, cipi/api 1.39+ and the dbs-manage ability.

  cipi-cli db unlink shop_reports`,
	Example: `  cipi-cli db unlink shop_reports
  cipi-cli prod db unlink shop_reports --engine pgsql`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		engine, _ := cmd.Flags().GetString("engine")

		client, err := mustClient()
		if err != nil {
			return err
		}

		path := dbAPIPath(args[0], "/link")
		if engine != "" {
			path += "?engine=" + url.QueryEscape(engine)
		}
		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Delete(path, nil, &result); err != nil {
			output.Error("Failed to unlink the database: %s", featureErrorHint(err, dbLinkMinAPI, "dbs-manage"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}
		output.Success("Database '%s' is no longer linked to an app", args[0])
		fmt.Println()
		return nil
	},
}

// dbLinkMinAPI is the cipi/api release that links databases to apps and takes
// uploaded dumps; the server also needs Cipi 5.9.0 (503 on an older core).
const dbLinkMinAPI = "1.39.0"

func dbAPIPath(name, suffix string) string {
	return "/api/dbs/" + url.PathEscape(name) + suffix
}

var dbDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a database permanently (API < 1.19 only)",
	Long: `Permanently delete a database. This cannot be undone.

Prompts for confirmation unless -y / --yes is passed.
Consider 'cipi-cli db backup <name>' first.

Cipi API 1.19+ removed database deletion: on those servers run
'cipi db delete <name>' on the host instead.

  cipi-cli db delete mydb
  cipi-cli db delete mydb -y`,
	Example: `  cipi-cli db delete mydb
  cipi-cli db delete mydb -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Delete database '%s'? This cannot be undone.", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Deleting database '%s'...", args[0])
		if err := client.DoAsyncAndWait("DELETE", fmt.Sprintf("/api/dbs/%s", args[0]), nil); err != nil {
			var apiErr *api.APIError
			if errors.As(err, &apiErr) && (apiErr.IsRouteNotFound() || apiErr.Status == http.StatusMethodNotAllowed) {
				output.Error("Failed to delete database: Cipi API 1.19+ no longer deletes databases — run 'cipi db delete %s' on the host", args[0])
				return err
			}
			output.Error("Failed to delete database: %s", err)
			return err
		}

		output.Success("Database '%s' deleted successfully", args[0])
		fmt.Println()
		return nil
	},
}

var dbBackupCmd = &cobra.Command{
	Use:   "backup <name>",
	Short: "Create a compressed backup of a database",
	Long: `Create a compressed backup of a database on the server.

  cipi-cli db backup mydb
  cipi-cli prod db backup mydb`,
	Example: `  cipi-cli db backup mydb
  cipi-cli staging db backup mydb`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Creating backup of '%s'...", args[0])
		job, err := client.DoAsyncAndWaitResult("POST", fmt.Sprintf("/api/dbs/%s/backup", args[0]), nil)
		if err != nil {
			output.Error("Backup failed: %s", err)
			return err
		}

		output.Success("Backup of '%s' created successfully", args[0])
		if result, ok := jobResultMap(job); ok {
			if file, _ := result["file"].(string); file != "" {
				output.KeyValue(nil, "File", file)
				output.Dim.Printf("  Restore it: cipi-cli db restore %s --file %s\n", args[0], file)
			}
		}
		fmt.Println()
		return nil
	},
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <name>",
	Short: "Restore a database from a dump on the server or uploaded from here",
	Long: `Restore a database from a dump. Give where the dump is:

  --file <path>     a .sql.gz already on the server, e.g. the file
                    "cipi-cli db backup" printed
  --upload <file>   a local .sql or .sql.gz: it is sent to the server in
                    chunks, restored, and deleted there when the restore ends

Overwrites current data. Prompts for confirmation unless -y / --yes. Use
--engine when the name exists on both engines.

An upload is limited in size by the server (2 GB unless configured otherwise:
cipi-cli db upload-limits); a larger dump is copied to the server and
restored with --file. The content must match the name: a .sql.gz must be
gzip-compressed, a .sql must not. Interrupting the upload (Ctrl-C) cancels it
on the server.

--upload requires Cipi 5.9.0+, cipi/api 1.39+ and the dbs-manage ability.

  cipi-cli db restore mydb --file /home/cipi/backups/mydb_2026-03-19.sql.gz
  cipi-cli db restore mydb --upload ./mydb.sql.gz`,
	Example: `  cipi-cli db restore mydb --file /home/cipi/backups/mydb_2026-03-19.sql.gz
  cipi-cli db restore mydb --upload ./mydb.sql.gz -y
  cipi-cli prod db restore mydb --upload ./mydb.sql --engine pgsql`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name := args[0]
		yes, _ := cmd.Flags().GetBool("yes")
		file, _ := cmd.Flags().GetString("file")
		upload, _ := cmd.Flags().GetString("upload")
		engine, _ := cmd.Flags().GetString("engine")

		switch {
		case file == "" && upload == "":
			output.Error("Say where the dump is: --file <.sql.gz on the server> or --upload <local .sql or .sql.gz>")
			return fmt.Errorf("no dump specified")
		case file != "" && upload != "":
			output.Error("Pass either --file or --upload, not both")
			return fmt.Errorf("both --file and --upload specified")
		}

		source := file
		var size int64
		if upload != "" {
			var err error
			if size, err = checkDumpFile(upload); err != nil {
				output.Error("%s", err)
				return err
			}
			source = fmt.Sprintf("%s (%s, uploaded from this machine)", upload, humanBytes(size))
		}

		if !yes {
			if !output.Confirm(fmt.Sprintf("Restore database '%s' from %s? Current data will be overwritten.", name, source)) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		body := map[string]string{}
		if engine != "" {
			body["engine"] = engine
		}
		if upload == "" {
			body["file"] = file
			output.Info("Restoring '%s' from %s...", name, file)
			if err := client.DoAsyncAndWait("POST", dbAPIPath(name, "/restore"), body); err != nil {
				output.Error("Restore failed: %s", err)
				return err
			}
		} else {
			output.Info("Uploading %s (%s)...", upload, humanBytes(size))
			uploadID, err := uploadDump(client, upload, size)
			if err != nil {
				output.Error("Upload failed: %s", featureErrorHint(err, dbLinkMinAPI, "dbs-manage"))
				return err
			}
			body["upload_id"] = uploadID

			output.Info("Restoring '%s' from the uploaded dump...", name)
			async, err := client.DoAsync("POST", dbAPIPath(name, "/restore"), body)
			if err != nil {
				// No job was queued, so nothing will delete the dump: do it here.
				cancelDumpUpload(client, uploadID)
				output.Error("Restore failed: %s", featureErrorHint(err, dbLinkMinAPI, "dbs-manage"))
				return err
			}
			if async != nil {
				output.Info("Job dispatched: %s", async.JobIDString())
				if _, err := client.WaitForJob(async.JobIDString()); err != nil {
					output.Error("Restore failed: %s", err)
					return err
				}
			}
		}

		output.Success("Database '%s' restored successfully", name)
		fmt.Println()
		return nil
	},
}

var dbPasswordCmd = &cobra.Command{
	Use:   "password <name>",
	Short: "Regenerate database password and update .env",
	Long: `Regenerate the database password and update the linked application's .env.

Prompts for confirmation unless -y / --yes is passed.

  cipi-cli db password mydb
  cipi-cli db password mydb -y`,
	Example: `  cipi-cli db password mydb
  cipi-cli db password mydb -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Regenerate password for database '%s'?", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Regenerating password for '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/dbs/%s/password", args[0]), nil); err != nil {
			output.Error("Password regeneration failed: %s", err)
			return err
		}

		output.Success("Password for '%s' regenerated and .env updated", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	dbDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	dbRestoreCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	dbPasswordCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	dbCreateCmd.Flags().String("engine", "", "Database engine (mariadb|pgsql)")
	dbCreateCmd.Flags().String("app", "", "Show the database as this app's (Cipi 5.9.0+ / API 1.39+)")
	dbListCmd.Flags().String("engine", "", "Filter by engine (mariadb|pgsql)")
	dbLinkCmd.Flags().String("engine", "", "Database engine, when the name exists on both (mariadb|pgsql)")
	dbUnlinkCmd.Flags().String("engine", "", "Database engine, when the name exists on both (mariadb|pgsql)")
	dbRestoreCmd.Flags().String("file", "", "Path of a .sql.gz dump on the server")
	dbRestoreCmd.Flags().String("upload", "", "Local .sql or .sql.gz dump to upload and restore (Cipi 5.9.0+ / API 1.39+)")
	dbRestoreCmd.Flags().String("engine", "", "Database engine, when the name exists on both (mariadb|pgsql)")

	dbCmd.AddCommand(dbEnginesCmd, dbInstallCmd, dbListCmd, dbCreateCmd, dbDeleteCmd, dbLinkCmd, dbUnlinkCmd, dbBackupCmd, dbRestoreCmd, dbUploadLimitsCmd, dbPasswordCmd)
	rootCmd.AddCommand(dbCmd)
}

// dbEnginesData is GET /api/dbs/engines. server and label: cipi/api 1.43+.
type dbEnginesData struct {
	Default *string `json:"default"`
	Engines []struct {
		Engine  string `json:"engine"`
		Status  string `json:"status"`
		Port    *int   `json:"port"`
		Default bool   `json:"default"`
		Server  string `json:"server,omitempty"`
		Label   string `json:"label,omitempty"`
	} `json:"engines"`
}

func printDBEngines(data dbEnginesData) {
	if len(data.Engines) == 0 {
		output.Warn("No database engine is installed")
		return
	}
	t := output.NewTable("ENGINE", "SERVER", "STATUS", "PORT", "DEFAULT")
	for _, e := range data.Engines {
		server := e.Label
		if server == "" {
			server = map[string]string{"mariadb": "MariaDB", "pgsql": "PostgreSQL"}[e.Engine]
		}
		port := "—"
		if e.Port != nil {
			port = fmt.Sprint(*e.Port)
		}
		def := ""
		if e.Default || (data.Default != nil && *data.Default == e.Engine) {
			def = output.Green.Sprint("✓")
		}
		t.Row(e.Engine, dash(server), e.Status, port, def)
	}
	t.Flush()
	fmt.Println()
}
