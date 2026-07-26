package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var dbCmd = &cobra.Command{
	Use:     "db",
	Aliases: []string{"database", "dbs"},
	Short:   "Manage databases",
	Long: `Create, backup, restore, and manage MySQL databases on the selected server.

  cipi-cli db list
  cipi-cli db create mydb
  cipi-cli db backup mydb
  cipi-cli db restore mydb
  cipi-cli db password mydb
  cipi-cli db delete mydb

` + multiServerTip,
	Example: `  cipi-cli db list
  cipi-cli prod db list
  cipi-cli db create mydb
  cipi-cli db backup mydb
  cipi-cli db restore mydb -y
  cipi-cli db password mydb -y
  cipi-cli db delete mydb -y`,
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
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		dbs, err := client.ListDatabases()
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
		t := output.NewTable("NAME", "SIZE")
		for _, db := range dbs {
			t.Row(
				str(db, "name"),
				str(db, "size"),
			)
		}
		t.Flush()
		output.Dim.Printf("  Total: %d database(s)\n\n", len(dbs))
		return nil
	},
}

var dbCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create a new database",
	Long: `Create a new MySQL database on the selected server.

  cipi-cli db create mydb
  cipi-cli prod db create mydb`,
	Example: `  cipi-cli db create mydb
  cipi-cli staging db create client_a`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		body := map[string]string{
			"name": args[0],
		}

		output.Info("Creating database '%s'...", args[0])
		if err := client.DoAsyncAndWait("POST", "/api/dbs", body); err != nil {
			output.Error("Failed to create database: %s", err)
			return err
		}

		output.Success("Database '%s' created successfully", args[0])
		fmt.Println()
		return nil
	},
}

var dbDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete a database permanently",
	Long: `Permanently delete a database. This cannot be undone.

Prompts for confirmation unless -y / --yes is passed.
Consider 'cipi-cli db backup <name>' first.

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
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/dbs/%s/backup", args[0]), nil); err != nil {
			output.Error("Backup failed: %s", err)
			return err
		}

		output.Success("Backup of '%s' created successfully", args[0])
		fmt.Println()
		return nil
	},
}

var dbRestoreCmd = &cobra.Command{
	Use:   "restore <name>",
	Short: "Restore a database from backup",
	Long: `Restore a database from its latest backup on the server.

Overwrites current data. Prompts for confirmation unless -y / --yes.

  cipi-cli db restore mydb
  cipi-cli db restore mydb -y`,
	Example: `  cipi-cli db restore mydb
  cipi-cli db restore mydb -y`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes {
			if !output.Confirm(fmt.Sprintf("Restore database '%s' from backup? Current data will be overwritten.", args[0])) {
				output.Warn("Aborted")
				return nil
			}
		}

		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Restoring '%s' from backup...", args[0])
		if err := client.DoAsyncAndWait("POST", fmt.Sprintf("/api/dbs/%s/restore", args[0]), nil); err != nil {
			output.Error("Restore failed: %s", err)
			return err
		}

		output.Success("Database '%s' restored successfully", args[0])
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

	dbCmd.AddCommand(dbListCmd, dbCreateCmd, dbDeleteCmd, dbBackupCmd, dbRestoreCmd, dbPasswordCmd)
	rootCmd.AddCommand(dbCmd)
}
