package cmd

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsBasicAuthCmd = &cobra.Command{
	Use:     "basicauth",
	Aliases: []string{"basic-auth"},
	Short:   "Manage HTTP Basic Auth on an app",
	Long: `Enable, disable, or inspect Nginx HTTP Basic Auth for an application.

  cipi-cli apps basicauth status myapp
  cipi-cli apps basicauth enable myapp --user admin
  cipi-cli apps basicauth disable myapp`,
}

var appsBasicAuthStatusCmd = &cobra.Command{
	Use:   "status <name>",
	Short: "Show HTTP Basic Auth status",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/basicauth"), &result); err != nil {
			// cipi/api before 1.43 on Cipi 5.11–5.12: the command printed the
			// state and exited 1, and the API answered that output as an error.
			enabled, users, ok := basicAuthStateFromText(err.Error())
			if !ok {
				output.Error("Failed to read basic auth status: %s", plainCLIText(err.Error()))
				return err
			}
			result.Data = map[string]interface{}{"enabled": enabled, "users": users}
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Header(fmt.Sprintf("Basic Auth: %s", args[0]))
		printBasicAuthStatus(result.Data)
		return nil
	},
}

var appsBasicAuthEnableCmd = &cobra.Command{
	Use:   "enable <name>",
	Short: "Enable HTTP Basic Auth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		user, _ := cmd.Flags().GetString("user")
		password, _ := cmd.Flags().GetString("password")

		body := map[string]string{}
		if user != "" {
			body["user"] = user
		}
		if password != "" {
			body["password"] = password
		}

		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data map[string]interface{} `json:"data"`
		}
		if err := client.Post(appAPIPath(args[0], "/basicauth/enable"), body, &result); err != nil {
			output.Error("Failed to enable basic auth: %s", plainCLIText(err.Error()))
			return err
		}

		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}

		output.Success("HTTP Basic Auth enabled for '%s'", args[0])
		if pwd, ok := result.Data["password"].(string); ok && pwd != "" {
			output.KeyValue(nil, "Password", pwd)
		}
		// cipi/api 1.43+: Cipi 5.11–5.12 enabled it but stopped before printing the credentials.
		if notice, ok := result.Data["notice"].(string); ok && notice != "" {
			output.Warn("%s", notice)
		}
		fmt.Println()
		return nil
	},
}

var appsBasicAuthDisableCmd = &cobra.Command{
	Use:   "disable <name>",
	Short: "Disable HTTP Basic Auth",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		if err := client.Post(appAPIPath(args[0], "/basicauth/disable"), nil, nil); err != nil {
			output.Error("Failed to disable basic auth: %s", plainCLIText(err.Error()))
			return err
		}

		output.Success("HTTP Basic Auth disabled for '%s'", args[0])
		fmt.Println()
		return nil
	},
}

// printBasicAuthStatus prints {enabled, users, agent} (agent: cipi/api 1.43+).
func printBasicAuthStatus(data map[string]interface{}) {
	enabled, _ := data["enabled"].(bool)
	if enabled {
		output.KeyValue(nil, "Status", output.Green.Sprint("enabled"))
	} else {
		output.KeyValue(nil, "Status", output.Dim.Sprint("disabled"))
	}
	if users, ok := data["users"].([]interface{}); ok && len(users) > 0 {
		names := make([]string, 0, len(users))
		for _, u := range users {
			if s, ok := u.(string); ok && s != "" {
				names = append(names, s)
			}
		}
		output.KeyValue(nil, "Users", strings.Join(names, ", "))
	} else if users, ok := data["users"].([]string); ok && len(users) > 0 {
		output.KeyValue(nil, "Users", strings.Join(users, ", "))
	}
	if agent, ok := data["agent"].(map[string]interface{}); ok {
		prefix, _ := agent["public_prefix"].(string)
		mode, _ := agent["mode"].(string)
		switch {
		case prefix != "":
			output.KeyValue(nil, "Agent", prefix+" left out — cipi/agent checks its own tokens")
		case mode == "protected":
			output.KeyValue(nil, "Agent", "behind basic auth")
		}
	}
	fmt.Println()
}

var (
	bareANSI        = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]|\[\d{1,3}(?:;\d{1,3})*m`)
	basicAuthStatus = regexp.MustCompile(`Basic auth\s+—\s+\S+\s+Status\s+(enabled|disabled)\b`)
	basicAuthUsers  = regexp.MustCompile(`\bUsers\s+([A-Za-z0-9._-]+(?:,[A-Za-z0-9._-]+)*)`)
)

// plainCLIText is a message that came from the Cipi CLI on the server without
// its terminal colours — escaped or left bare ("[1m") — or the rule under a
// title, on one line.
func plainCLIText(text string) string {
	text = bareANSI.ReplaceAllString(text, "")
	text = strings.ReplaceAll(text, "━", "")
	return strings.Join(strings.Fields(text), " ")
}

// basicAuthStateFromText reads the state out of the output of "cipi basicauth
// status" that an API before 1.43 answered as an error.
func basicAuthStateFromText(text string) (enabled bool, users []string, ok bool) {
	plain := plainCLIText(text)
	m := basicAuthStatus.FindStringSubmatch(plain)
	if m == nil {
		return false, nil, false
	}
	users = []string{}
	if u := basicAuthUsers.FindStringSubmatch(plain); u != nil {
		users = strings.Split(u[1], ",")
	}
	return m[1] == "enabled", users, true
}

func init() {
	appsBasicAuthEnableCmd.Flags().String("user", "", "Username (default: admin)")
	appsBasicAuthEnableCmd.Flags().String("password", "", "Password (auto-generated when omitted)")

	appsBasicAuthCmd.AddCommand(appsBasicAuthStatusCmd, appsBasicAuthEnableCmd, appsBasicAuthDisableCmd)
	appsCmd.AddCommand(appsBasicAuthCmd)
}
