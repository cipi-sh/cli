package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

var appsEnvCmd = &cobra.Command{
	Use:   "env",
	Short: "Manage Laravel app .env variables",
	Long: `Read, merge or reorder .env key/value pairs for a Laravel application.

Requires Cipi 5.0.3+ and the apps-env token ability; reordering needs
Cipi 5.13.0+ and cipi/api 1.43+.

  cipi-cli apps env show myapp
  cipi-cli apps env set myapp --set APP_DEBUG=false --unset OLD_KEY
  cipi-cli apps env order myapp DB_HOST DB_PORT --after APP_URL`,
}

var appsEnvShowCmd = &cobra.Command{
	Use:   "show <name>",
	Short: "Show .env variables in file order (secrets redacted)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}

		var result struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get(appAPIPath(args[0], "/env"), &result); err != nil {
			output.Error("Failed to read env: %s", apiErrorHint(err, "5.0.3", "apps-env"))
			return err
		}

		if jsonFlag {
			output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
			return nil
		}

		output.Header(fmt.Sprintf(".env: %s", args[0]))
		keys, vals, err := envVarsInOrder(result.Data)
		if err != nil {
			output.Error("Unexpected answer: %s", err)
			return err
		}
		if len(keys) == 0 {
			output.Warn("The file is empty")
			return nil
		}
		for _, k := range keys {
			output.KeyValue(nil, k, vals[k])
		}
		fmt.Println()
		return nil
	},
}

var appsEnvSetCmd = &cobra.Command{
	Use:   "set <name>",
	Short: "Merge, unset or reorder .env keys",
	Long: `Update .env keys without replacing the whole file.

--order lists every key the file has after --set and --unset, each once, in
the order they should be in (Cipi 5.13.0+, cipi/api 1.43+); comments above a
key move with it. "apps env order" builds that list for you.

  cipi-cli apps env set myapp --set MAIL_HOST=smtp.example.com --unset OLD_KEY`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		setPairs, _ := cmd.Flags().GetStringArray("set")
		unsetKeys, _ := cmd.Flags().GetStringArray("unset")
		order, _ := cmd.Flags().GetStringSlice("order")

		if len(setPairs) == 0 && len(unsetKeys) == 0 && len(order) == 0 {
			output.Error("Pass at least one --set KEY=VALUE, --unset KEY or --order KEY,KEY,…")
			return fmt.Errorf("no env changes specified")
		}

		body := map[string]interface{}{}
		if len(setPairs) > 0 {
			setMap, err := parseKeyValuePairs(setPairs)
			if err != nil {
				output.Error("%s", err)
				return err
			}
			body["set"] = setMap
		}
		if len(unsetKeys) > 0 {
			body["unset"] = unsetKeys
		}
		if len(order) > 0 {
			if dup := firstDuplicate(order); dup != "" {
				output.Error("%s is twice in --order", dup)
				return fmt.Errorf("duplicate key in --order")
			}
			body["order"] = order
		}

		return putEnv(args[0], body, len(order) > 0)
	},
}

var appsEnvOrderCmd = &cobra.Command{
	Use:   "order <name> <KEY>...",
	Short: "Move .env keys (Cipi 5.13.0+)",
	Long: `Move keys of the .env: the keys given go, in that order, to the top of the
file — or right after (--after) or before (--before) another key. Every other
key keeps its place. Comments above a key move with it.

Reads the file's current order first, then sends the whole order
(PUT /api/apps/{name}/env with "order"). Requires Cipi 5.13.0+, cipi/api
1.43+ and the apps-env ability.`,
	Example: `  cipi-cli apps env order myapp APP_NAME APP_ENV APP_KEY
  cipi-cli apps env order myapp DB_HOST DB_PORT --after APP_URL
  cipi-cli prod apps env order myapp SENTRY_DSN --before LOG_CHANNEL`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		app, move := args[0], args[1:]
		after, _ := cmd.Flags().GetString("after")
		before, _ := cmd.Flags().GetString("before")
		if after != "" && before != "" {
			output.Error("Use either --after or --before, not both")
			return fmt.Errorf("both --after and --before")
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		var current struct {
			Data json.RawMessage `json:"data"`
		}
		if err := client.Get(appAPIPath(app, "/env"), &current); err != nil {
			output.Error("Failed to read env: %s", apiErrorHint(err, "5.0.3", "apps-env"))
			return err
		}
		keys, _, err := envVarsInOrder(current.Data)
		if err != nil {
			output.Error("Unexpected answer: %s", err)
			return err
		}

		order, err := envMoveKeys(keys, move, after, before)
		if err != nil {
			output.Error("%s", err)
			return err
		}
		if strings.Join(order, "\x00") == strings.Join(keys, "\x00") {
			output.Info("Already in that order — nothing to change")
			return nil
		}

		return putEnv(app, map[string]interface{}{"order": order}, true)
	},
}

// putEnv sends a change of the .env and reports it. ordered: the body has an
// order, and an API before 1.43 that saved the values but dropped it is told
// apart from one that kept it.
func putEnv(app string, body map[string]interface{}, ordered bool) error {
	client, err := mustClient()
	if err != nil {
		return err
	}

	var result struct {
		Data json.RawMessage `json:"data"`
	}
	if err := client.Put(appAPIPath(app, "/env"), body, &result); err != nil {
		msg := apiErrorHint(err, "5.0.3", "apps-env")
		if ordered && isHTTPStatus(err, 422) && !strings.Contains(strings.ToLower(msg), "order") {
			msg += " — reordering the .env needs cipi/api 1.43+ and Cipi 5.13.0+ on the server (cipi self-update && cipi api update)"
		}
		output.Error("Failed to update env: %s", msg)
		return err
	}

	if jsonFlag {
		output.PrintJSON(map[string]json.RawMessage{"data": result.Data})
		return nil
	}

	if ordered {
		if want, ok := body["order"].([]string); ok {
			if got, _, err := envVarsInOrder(result.Data); err == nil && strings.Join(got, "\x00") != strings.Join(want, "\x00") {
				output.Warn("Saved, but the server did not keep the new order — it needs cipi/api 1.43+ and Cipi 5.13.0+")
				fmt.Println()
				return nil
			}
		}
	}

	output.Success("Updated .env for '%s'", app)
	output.Dim.Println("  Applies on the next deploy, or after config:cache.")
	fmt.Println()
	return nil
}

// envVarsInOrder reads data.vars (data.variables on some older answers) of
// GET|PUT /api/apps/{name}/env keeping the order of the file, which a Go map
// would lose.
func envVarsInOrder(data json.RawMessage) ([]string, map[string]string, error) {
	var wrapper map[string]json.RawMessage
	if err := json.Unmarshal(data, &wrapper); err != nil {
		return nil, nil, err
	}
	raw, ok := wrapper["vars"]
	if !ok {
		raw, ok = wrapper["variables"]
	}
	if !ok || len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" || string(bytes.TrimSpace(raw)) == "[]" {
		return []string{}, map[string]string{}, nil
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil, nil, fmt.Errorf("vars is not an object")
	}
	keys := []string{}
	vals := map[string]string{}
	for dec.More() {
		kt, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, _ := kt.(string)
		var v interface{}
		if err := dec.Decode(&v); err != nil {
			return nil, nil, err
		}
		if _, seen := vals[key]; !seen {
			keys = append(keys, key)
		}
		switch t := v.(type) {
		case nil:
			vals[key] = ""
		case string:
			vals[key] = t
		default:
			vals[key] = formatFieldValue(t)
		}
	}
	return keys, vals, nil
}

// envMoveKeys returns the whole order of the file with the keys of move in
// that order at the top, or right after `after`, or right before `before`;
// every other key keeps its place.
func envMoveKeys(keys, move []string, after, before string) ([]string, error) {
	have := make(map[string]bool, len(keys))
	for _, k := range keys {
		have[k] = true
	}
	moving := make(map[string]bool, len(move))
	for _, k := range move {
		if !have[k] {
			return nil, fmt.Errorf("%s is not in the .env", k)
		}
		if moving[k] {
			return nil, fmt.Errorf("%s is given twice", k)
		}
		moving[k] = true
	}
	anchor := after + before
	if anchor != "" {
		if !have[anchor] {
			return nil, fmt.Errorf("%s is not in the .env", anchor)
		}
		if moving[anchor] {
			return nil, fmt.Errorf("%s is one of the keys being moved", anchor)
		}
	}

	rest := make([]string, 0, len(keys))
	for _, k := range keys {
		if !moving[k] {
			rest = append(rest, k)
		}
	}
	if anchor == "" {
		return append(append([]string{}, move...), rest...), nil
	}
	out := make([]string, 0, len(keys))
	for _, k := range rest {
		if k == before {
			out = append(out, move...)
		}
		out = append(out, k)
		if k == after {
			out = append(out, move...)
		}
	}
	return out, nil
}

func firstDuplicate(list []string) string {
	seen := make(map[string]bool, len(list))
	for _, v := range list {
		if seen[v] {
			return v
		}
		seen[v] = true
	}
	return ""
}

func init() {
	appsEnvSetCmd.Flags().StringArray("set", nil, "Set KEY=VALUE (repeatable)")
	appsEnvSetCmd.Flags().StringArray("unset", nil, "Remove KEY (repeatable)")
	appsEnvSetCmd.Flags().StringSlice("order", nil, "Every key of the file, in order, after --set/--unset (Cipi 5.13.0+)")

	appsEnvOrderCmd.Flags().String("after", "", "Put the keys right after this one")
	appsEnvOrderCmd.Flags().String("before", "", "Put the keys right before this one")

	appsEnvCmd.AddCommand(appsEnvShowCmd, appsEnvSetCmd, appsEnvOrderCmd)
	appsCmd.AddCommand(appsEnvCmd)
}
