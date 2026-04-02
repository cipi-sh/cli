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
}

var jobsShowCmd = &cobra.Command{
	Use:   "show [id]",
	Short: "Show the status of an async job",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := api.NewClient()
		if err != nil {
			output.Error("%s", err)
			return err
		}

		var job api.JobStatus
		if err := client.Get(fmt.Sprintf("/api/jobs/%s", args[0]), &job); err != nil {
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
	Use:   "wait [id]",
	Short: "Wait for a job to complete",
	Args:  cobra.ExactArgs(1),
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
