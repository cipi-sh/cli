package cmd

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/cipi-sh/cli/internal/api"
	"github.com/cipi-sh/cli/internal/output"
	"github.com/spf13/cobra"
)

// An app's own nginx directives came with Cipi 5.14.0 and cipi/api 1.44
// (abilities nginx-view, nginx-manage).
const nginxMinAPI = "1.44.0"

// The server's own limit for the text of one app.
const nginxMaxBytes = 32768

// nginxDirectives is the data of GET /api/apps/{app}/nginx.
type nginxDirectives struct {
	App       string `json:"app"`
	Config    string `json:"config"`
	Bytes     int    `json:"bytes"`
	MaxBytes  int    `json:"max_bytes"`
	Active    bool   `json:"active"`
	Suspended bool   `json:"suspended"`
}

type nginxResponse struct {
	Data nginxDirectives `json:"data"`
}

func nginxPath(app string) string {
	return "/api/apps/" + url.PathEscape(app) + "/nginx"
}

// normalizeNginxText is what both sides compare and send: LF line ends, one
// newline at the end of a text that has any. Only comments and blank lines
// means nothing to include.
func normalizeNginxText(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	meaningful := false
	for _, line := range strings.Split(text, "\n") {
		if t := strings.TrimSpace(line); t != "" && !strings.HasPrefix(t, "#") {
			meaningful = true
			break
		}
	}
	if !meaningful {
		return ""
	}
	return strings.TrimRight(text, "\n") + "\n"
}

func validateNginxText(text string) error {
	if len(text) > nginxMaxBytes {
		return fmt.Errorf("the directives are too long: %d bytes, %d at most", len(text), nginxMaxBytes)
	}
	for _, r := range text {
		if (r < 32 && r != '\t' && r != '\n') || r == 127 {
			return errors.New("the directives hold a control character")
		}
	}
	return nil
}

// nginxRefusal is what a 422 says: the reasons, one per line, or its message.
func nginxRefusal(err error) []string {
	var apiErr *api.APIError
	if errors.As(err, &apiErr) && apiErr.Status == 422 {
		if len(apiErr.Reasons) > 0 {
			return apiErr.Reasons
		}
		return []string{apiErr.Error()}
	}
	return nil
}

// putNginx replaces the directives. A refused text is reported line by line
// and returned as an error: nothing changed on the server.
func putNginx(client *api.Client, app, text string) (nginxDirectives, error) {
	var result nginxResponse
	var err error
	if text == "" {
		err = client.Delete(nginxPath(app), nil, &result)
	} else {
		err = client.Put(nginxPath(app), map[string]string{"config": text}, &result)
	}
	if err == nil {
		return result.Data, nil
	}
	if reasons := nginxRefusal(err); reasons != nil {
		output.Error("Refused — nothing changed on the server:")
		for _, reason := range reasons {
			fmt.Println("    " + reason)
		}
		return result.Data, err
	}
	output.Error("Failed to save the directives: %s", featureErrorHint(err, nginxMinAPI, "nginx-manage"))
	return result.Data, err
}

var nginxCmd = &cobra.Command{
	Use:   "nginx",
	Short: "An app's own nginx directives: show, edit, set, reset",
	Long: `The directives an app adds to its own nginx server block: headers, caching,
limits, rewrites, location blocks. Cipi keeps them and includes them in the
app's vhost, so they survive every change of it (aliases, SSL, PHP, routing).

Every change is tested with "nginx -t" on the server before nginx reloads; a
refused text changes nothing, and the reasons are shown line by line.

From the API a subset of nginx's language is taken: no access_log or error_log
with a path, include, listen, server_name, *_pass, certificates; root and
alias only under the app's home. Reverse proxies and redirects have their own
commands (cipi-cli proxies, cipi-cli redirect); the whole language is
"cipi nginx edit <app>" on the server.

Requires Cipi 5.14.0+, cipi/api 1.44+ and the nginx-view / nginx-manage
abilities.

  cipi-cli nginx show shop
  cipi-cli nginx edit shop
  cipi-cli nginx set shop --file nginx.conf
  cipi-cli nginx reset shop`,
}

var nginxShowCmd = &cobra.Command{
	Use:   "show <app>",
	Short: "Print the app's directives",
	Example: `  cipi-cli nginx show shop
  cipi-cli nginx show shop > shop.nginx.conf
  cipi-cli nginx show shop --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		client, err := mustClient()
		if err != nil {
			return err
		}
		var result nginxResponse
		if err := client.Get(nginxPath(args[0]), &result); err != nil {
			output.Error("Failed to read the directives: %s", featureErrorHint(err, nginxMinAPI, "nginx-view"))
			return err
		}
		if jsonFlag {
			output.PrintJSON(result)
			return nil
		}
		// The text alone on stdout, so it can be redirected to a file.
		if strings.TrimSpace(result.Data.Config) == "" {
			fmt.Fprintf(os.Stderr, "%s has no directives of its own — add them with: cipi-cli nginx edit %s\n", args[0], args[0])
			return nil
		}
		fmt.Print(strings.TrimRight(result.Data.Config, "\n") + "\n")
		if !result.Data.Active {
			fmt.Fprintln(os.Stderr, "(saved, not in the vhost right now — a suspended app?)")
		}
		return nil
	},
}

var nginxSetCmd = &cobra.Command{
	Use:   "set <app> --file <path>",
	Short: "Replace the app's directives with a file's content",
	Long: `Replace the app's directives with the content of a file ("-" reads stdin).
A file with only comments or blank lines removes them.

  cipi-cli nginx set shop --file shop.nginx.conf
  cat shop.nginx.conf | cipi-cli nginx set shop --file -`,
	Example: `  cipi-cli nginx set shop --file shop.nginx.conf`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path, _ := cmd.Flags().GetString("file")
		if path == "" {
			err := errors.New("--file is required (a path, or - for stdin)")
			output.Error("%s", err)
			return err
		}
		var raw []byte
		var err error
		if path == "-" {
			raw, err = io.ReadAll(io.LimitReader(os.Stdin, nginxMaxBytes+1))
		} else {
			raw, err = os.ReadFile(path)
		}
		if err != nil {
			output.Error("Cannot read %s: %s", path, err)
			return err
		}
		text := normalizeNginxText(string(raw))
		if err := validateNginxText(text); err != nil {
			output.Error("%s", err)
			return err
		}

		client, err := mustClient()
		if err != nil {
			return err
		}
		if _, err := putNginx(client, args[0], text); err != nil {
			return err
		}
		if text == "" {
			output.Success("Directives of %s removed", args[0])
		} else {
			output.Success("Directives of %s saved and live (%d bytes)", args[0], len(text))
		}
		fmt.Println()
		return nil
	},
}

var nginxResetCmd = &cobra.Command{
	Use:     "reset <app>",
	Aliases: []string{"clear", "unset"},
	Short:   "Remove the app's directives",
	Example: `  cipi-cli nginx reset shop -y`,
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !output.Confirm(fmt.Sprintf("Remove the nginx directives of %s?", args[0])) {
			output.Info("Nothing changed")
			return nil
		}
		client, err := mustClient()
		if err != nil {
			return err
		}
		if _, err := putNginx(client, args[0], ""); err != nil {
			return err
		}
		output.Success("Directives of %s removed", args[0])
		fmt.Println()
		return nil
	},
}

const nginxTemplate = `# nginx directives of this app — included in its server block.
# Lines starting with # are comments. Save with nothing else to have none.
#
# client_max_body_size 512M;
# location ^~ /downloads/ {
#     add_header Cache-Control "public, max-age=31536000, immutable";
# }
`

// editorCommand is $VISUAL, then $EDITOR, then vi — with its own arguments
// ("code -w") and the file last.
func editorCommand(file string) *exec.Cmd {
	editor := strings.TrimSpace(os.Getenv("VISUAL"))
	if editor == "" {
		editor = strings.TrimSpace(os.Getenv("EDITOR"))
	}
	if editor == "" {
		editor = "vi"
	}
	parts := strings.Fields(editor)
	cmd := exec.Command(parts[0], append(parts[1:], file)...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd
}

var nginxEditCmd = &cobra.Command{
	Use:   "edit <app>",
	Short: "Edit the app's directives in your editor",
	Long: `Open the app's directives in $VISUAL or $EDITOR (vi without them), on this
computer. When the editor closes, a changed text is sent to the server, tested
with "nginx -t" and made live. Refused, nothing changes there: the reasons
are shown and the editor can be opened again on your text.`,
	Example: `  cipi-cli nginx edit shop
  EDITOR="code -w" cipi-cli prod nginx edit shop`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		app := args[0]
		client, err := mustClient()
		if err != nil {
			return err
		}
		var current nginxResponse
		if err := client.Get(nginxPath(app), &current); err != nil {
			output.Error("Failed to read the directives: %s", featureErrorHint(err, nginxMinAPI, "nginx-view"))
			return err
		}
		before := normalizeNginxText(current.Data.Config)

		tmp, err := os.CreateTemp("", "cipi-nginx-"+app+"-*.conf")
		if err != nil {
			output.Error("Cannot create a temporary file: %s", err)
			return err
		}
		file := tmp.Name()
		defer os.Remove(file)
		start := before
		if start == "" {
			start = nginxTemplate
		}
		if _, err := tmp.WriteString(start); err != nil {
			tmp.Close()
			output.Error("Cannot write %s: %s", file, err)
			return err
		}
		tmp.Close()

		for {
			if err := editorCommand(file).Run(); err != nil {
				output.Error("The editor exited with an error — nothing changed: %s", err)
				return err
			}
			raw, err := os.ReadFile(file)
			if err != nil {
				output.Error("Cannot read %s: %s", file, err)
				return err
			}
			text := normalizeNginxText(string(raw))
			if text == before {
				output.Info("No change")
				return nil
			}
			err = validateNginxText(text)
			if err != nil {
				output.Error("%s", err)
			} else if _, err = putNginx(client, app, text); err == nil {
				if text == "" {
					output.Success("Directives of %s removed", app)
				} else {
					output.Success("Directives of %s saved and live", app)
				}
				fmt.Println()
				return nil
			} else if nginxRefusal(err) == nil {
				return err // not about the text: opening the editor again would not help
			}
			if !output.Confirm("Open the editor again?") {
				return err
			}
		}
	},
}

func init() {
	nginxSetCmd.Flags().StringP("file", "f", "", "file with the directives (- for stdin)")
	nginxResetCmd.Flags().BoolP("yes", "y", false, "do not ask for confirmation")
	nginxCmd.AddCommand(nginxShowCmd, nginxEditCmd, nginxSetCmd, nginxResetCmd)
	rootCmd.AddCommand(nginxCmd)
}
