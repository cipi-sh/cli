package cmd

import (
	"fmt"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var jobsCmd = &cobra.Command{
	Use:     "jobs",
	Aliases: []string{"job"},
	Short:   "Inspect async job status",
	Long: `Many Cipi API actions run as async jobs. Most CLI commands wait for them
automatically; use these subcommands to inspect or wait on a job by ID.

  cipi-cli jobs show <id>
  cipi-cli jobs wait <id>

` + multiServerTip,
	Example: `  cipi-cli jobs show 42
  cipi-cli jobs wait 42
  cipi-cli prod jobs show 42`,
}

var jobsShowCmd = &cobra.Command{
	Use:   "show <id>",
	Short: "Show the status of an async job",
	Long: `Show the current status of an async job by ID.

Statuses include pending, processing/running, completed/success, failed/error.

  cipi-cli jobs show 42
  cipi-cli prod jobs show 42`,
	Example: `  cipi-cli jobs show 42
  cipi-cli jobs show 42 --json
  cipi-cli staging jobs show 42`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		job, err := client.GetJob(args[0])
		if err != nil {
			output.Error("Failed to get job: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(job)
			return nil
		}

		output.Header("Job Status")
		output.KeyValue(nil, "ID", fmt.Sprintf("%v", job.ID))

		switch job.Status {
		case "completed", "success", "finished":
			output.KeyValue(nil, "Status", output.Green.Sprint(job.Status))
		case "failed", "error":
			output.KeyValue(nil, "Status", output.Red.Sprint(job.Status))
		case "pending", "processing", "running":
			output.KeyValue(nil, "Status", output.Yellow.Sprint(job.Status))
		default:
			output.KeyValue(nil, "Status", job.Status)
		}

		if job.Error != "" {
			output.KeyValue(nil, "Error", output.Red.Sprint(job.Error))
		}

		fmt.Println()
		return nil
	},
}

var jobsWaitCmd = &cobra.Command{
	Use:   "wait <id>",
	Short: "Wait for a job to complete",
	Long: `Poll an async job until it completes or fails, then exit.

  cipi-cli jobs wait 42
  cipi-cli prod jobs wait 42`,
	Example: `  cipi-cli jobs wait 42
  cipi-cli staging jobs wait 42 --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		output.Info("Waiting for job %s...", args[0])
		job, err := client.WaitForJob(args[0])
		if err != nil {
			output.Error("Job failed: %s", err)
			return err
		}

		if jsonFlag {
			output.PrintJSON(job)
			return nil
		}

		output.Success("Job %s completed", args[0])
		fmt.Println()
		return nil
	},
}

func init() {
	jobsCmd.AddCommand(jobsShowCmd, jobsWaitCmd)
	rootCmd.AddCommand(jobsCmd)
}
