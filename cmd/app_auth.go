package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsAuthCmd = &cobra.Command{
	Use:   "auth",
	Short: "Manage shared auth.json (Composer credentials)",
	Long: `Manage the shared auth.json file for Composer — not HTTP Basic Auth.

Requires Cipi 5.0.3+ and the apps-auth token ability.

  cipi-cli apps auth show myapp
  cipi-cli apps auth create myapp
  cipi-cli apps auth update myapp --file auth.json
  cipi-cli apps auth delete myapp`,
}

var appsAuthShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show shared auth.json",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/auth"), &result); err != nil {
			output.Error("Failed to read auth.json: %s", apiErrorHint(err, "5.0.3", "apps-auth"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("auth.json: %s", args[0]))
		printDataWrapper(result.Data)
		return nil
	},
}

var appsAuthCreateCmd = &cobra.Command{
	Use:   "create <name>",
	Short: "Create shared auth.json",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		force, _ := cmd.Flags().GetBool("force")

		client, err := mustClient()
		if err != nil {
			return err
		}

		body := map[string]interface{}{}
		if force {
			body["force"] = true
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post(appAPIPath(args[0], "/auth"), body, &result); err != nil {
			output.Error("Failed to create auth.json: %s", apiErrorHint(err, "5.0.3", "apps-auth"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Created auth.json for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var appsAuthUpdateCmd = &cobra.Command{
	Use:   "update <name>",
	Short: "Replace shared auth.json from a JSON file",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			output.Error("--file is required")
			return fmt.Errorf("missing --file")
		}

		doc, err := readJSONFile(file)
		if err != nil {
			output.Error("Failed to read JSON file: %s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Put(appAPIPath(args[0], "/auth"), doc, &result); err != nil {
			output.Error("Failed to update auth.json: %s", apiErrorHint(err, "5.0.3", "apps-auth"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("Updated auth.json for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

var appsAuthDeleteCmd = &cobra.Command{
	Use:   "delete <name>",
	Short: "Delete shared auth.json",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Delete auth.json for '%s'?", args[0])) {
			output.Warn("Aborted")
			return nil
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Delete(appAPIPath(args[0], "/auth"), nil, nil); err != nil {
			output.Error("Failed to delete auth.json: %s", apiErrorHint(err, "5.0.3", "apps-auth"))
			return err
		}

		output.Success("Deleted auth.json for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	appsAuthCreateCmd.Flags().Bool("force", false, "Overwrite if auth.json already exists")
	appsAuthUpdateCmd.Flags().String("file", "", "JSON file to upload")
	appsAuthDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")

	appsAuthCmd.AddCommand(appsAuthShowCmd, appsAuthCreateCmd, appsAuthUpdateCmd, appsAuthDeleteCmd)
	appsCmd.AddCommand(appsAuthCmd)
}
